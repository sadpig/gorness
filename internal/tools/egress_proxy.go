package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type EgressProxyOption func(*EgressProxy)

func WithProxyDialContext(fn DialContextFunc) EgressProxyOption {
	return func(p *EgressProxy) {
		p.customDial = fn
	}
}

// EgressProxy 是绑定在 127.0.0.1 随机临时端口上的本地域名白名单代理（支持 HTTP 转发与 HTTPS CONNECT 隧道），
// 配合 macOS Seatbelt 仅放行该代理端口，实现沙箱子进程出站域名的内核+代理双层精确管控。
type EgressProxy struct {
	listener       net.Listener
	server         *http.Server
	transport      *http.Transport
	port           int
	allowAll       bool
	allowLoopback  bool
	allowedDomains []string
	customDial     DialContextFunc

	mu         sync.Mutex
	closed     bool
	activeConn map[net.Conn]struct{}
}

func NewEgressProxy(allowedDomains []string, opts ...EgressProxyOption) (*EgressProxy, error) {
	p := &EgressProxy{
		activeConn: make(map[net.Conn]struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	seen := make(map[string]struct{}, len(allowedDomains))
	for _, raw := range allowedDomains {
		norm := normalizeDomainPattern(raw)
		if norm == "" {
			continue
		}
		if norm == "*" {
			p.allowAll = true
		}
		if norm == "127.0.0.1" || norm == "localhost" || norm == "::1" {
			p.allowLoopback = true
		}
		if _, exists := seen[norm]; !exists {
			seen[norm] = struct{}{}
			p.allowedDomains = append(p.allowedDomains, norm)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind egress proxy listener: %w", err)
	}
	p.listener = ln
	p.port = ln.Addr().(*net.TCPAddr).Port

	p.transport = &http.Transport{
		Proxy:                 nil,
		DialContext:           p.dialUpstream,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	p.server = &http.Server{
		Handler:           http.HandlerFunc(p.serveHTTP),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		_ = p.server.Serve(ln)
	}()

	return p, nil
}

func (p *EgressProxy) Port() int {
	if p == nil {
		return 0
	}
	return p.port
}

func (p *EgressProxy) Addr() string {
	if p == nil || p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

func (p *EgressProxy) URL() string {
	if p == nil || p.port == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", p.port)
}

func (p *EgressProxy) IsAllowed(hostOrAuthority string) bool {
	if p == nil {
		return false
	}
	host := extractHostname(hostOrAuthority)
	if host == "" {
		return false
	}
	if p.allowAll {
		return true
	}
	for _, pat := range p.allowedDomains {
		if strings.HasPrefix(pat, "*.") {
			base := strings.TrimPrefix(pat, "*.")
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
			continue
		}
		if host == pat || strings.HasSuffix(host, "."+pat) {
			return true
		}
	}
	return false
}

func (p *EgressProxy) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	conns := make([]net.Conn, 0, len(p.activeConn))
	for c := range p.activeConn {
		conns = append(conns, c)
	}
	p.activeConn = make(map[net.Conn]struct{})
	p.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

func (p *EgressProxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleForwardHTTP(w, r)
}

func (p *EgressProxy) handleForwardHTTP(w http.ResponseWriter, r *http.Request) {
	targetHost := r.URL.Host
	if targetHost == "" {
		targetHost = r.Host
	}
	if !p.IsAllowed(targetHost) {
		http.Error(w, fmt.Sprintf("egress blocked: domain %q is not in allowed_domains", extractHostname(targetHost)), http.StatusForbidden)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = targetHost
	}
	stripHopHeaders(outReq.Header)

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("egress upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	stripHopHeaders(resp.Header)
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *EgressProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	authority := r.Host
	if authority == "" {
		authority = r.URL.Host
	}
	if !p.IsAllowed(authority) {
		http.Error(w, fmt.Sprintf("egress blocked: domain %q is not in allowed_domains", extractHostname(authority)), http.StatusForbidden)
		return
	}

	targetAddr := authority
	if _, _, splitErr := net.SplitHostPort(targetAddr); splitErr != nil {
		targetAddr = net.JoinHostPort(targetAddr, "443")
	}

	upstreamConn, err := p.dialUpstream(r.Context(), "tcp", targetAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("egress connect failed: %v", err), http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = upstreamConn.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		_ = upstreamConn.Close()
		return
	}

	if !p.trackConn(clientConn) {
		_ = clientConn.Close()
		_ = upstreamConn.Close()
		return
	}
	if !p.trackConn(upstreamConn) {
		p.untrackConn(clientConn)
		_ = clientConn.Close()
		_ = upstreamConn.Close()
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if clientBuf != nil && clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		n, _ := clientBuf.Read(buffered)
		if n > 0 {
			_, _ = upstreamConn.Write(buffered[:n])
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstreamConn, clientConn)
		if cw, ok := upstreamConn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, upstreamConn)
		if cw, ok := clientConn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	wg.Wait()

	p.untrackConn(clientConn)
	p.untrackConn(upstreamConn)
	_ = clientConn.Close()
	_ = upstreamConn.Close()
}

func (p *EgressProxy) dialUpstream(ctx context.Context, network, addr string) (net.Conn, error) {
	if p.customDial != nil {
		return p.customDial(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses resolved for %s", host)
	}

	for _, ipAddr := range ips {
		if !p.allowLoopback && (ipAddr.IP.IsLoopback() || ipAddr.IP.IsUnspecified() || ipAddr.IP.IsLinkLocalUnicast()) {
			return nil, fmt.Errorf("egress blocked: %s resolved to restricted local address %s", host, ipAddr.IP.String())
		}
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func (p *EgressProxy) trackConn(c net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.activeConn[c] = struct{}{}
	return true
}

func (p *EgressProxy) untrackConn(c net.Conn) {
	p.mu.Lock()
	delete(p.activeConn, c)
	p.mu.Unlock()
}

func stripHopHeaders(h http.Header) {
	for _, k := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(k)
	}
}

func normalizeDomainPattern(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if slashIdx := strings.IndexByte(s, '/'); slashIdx >= 0 {
		s = s[:slashIdx]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	s = strings.TrimSuffix(s, ".")
	return s
}

func extractHostname(hostOrAuthority string) string {
	s := strings.ToLower(strings.TrimSpace(hostOrAuthority))
	if s == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSuffix(s, ".")
	return s
}
