package server

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// LoopbackGuard 强制执行本机回环同源守卫、Unix Domain Socket 放行与零 CORS 铁律：
// 1. 校验 Host 头防 DNS Rebinding（TCP 仅允许 127.0.0.1 / localhost / [::1]；本机 UDS 额外兼容无端口/unix 主机名）；
// 2. 拦截非同源 Sec-Fetch-Site（cross-site / same-site）、CORS 预检请求与 Origin: null（沙箱 iframe 透明源）；
// 3. 校验非空 Origin 与 Referer 必须匹配当前服务的 http://<loopback>:<port>；
// 4. 全站响应严禁输出任何 Access-Control-Allow-* 跨域头。
func LoopbackGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripCORSHeaders(w.Header())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")

		isUDS := isUnixDomainSocketPeer(r.RemoteAddr)
		hasBrowserFetchMeta := strings.TrimSpace(r.Header.Get("Origin")) != "" ||
			strings.TrimSpace(r.Header.Get("Referer")) != "" ||
			strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")) != ""

		_, reqPort, hostOK := parseLoopbackHostPort(r.Host)
		if !hostOK && isUDS && !hasBrowserFetchMeta && isAllowedUDSHost(r.Host) {
			reqPort = "80"
			hostOK = true
		}
		if !hostOK {
			rejectForbidden(w, "forbidden host header")
			return
		}

		if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs != "" && sfs != "same-origin" && sfs != "none" {
			rejectForbidden(w, "forbidden cross-site request")
			return
		}

		hasOrigin := false
		for _, rawOrigin := range r.Header.Values("Origin") {
			trimmed := strings.TrimSpace(rawOrigin)
			if trimmed == "" {
				continue
			}
			hasOrigin = true
			if !isSameLoopbackOriginHeader(trimmed, reqPort) {
				rejectForbidden(w, "forbidden cross-origin request")
				return
			}
		}

		for _, rawRef := range r.Header.Values("Referer") {
			trimmed := strings.TrimSpace(rawRef)
			if trimmed == "" {
				continue
			}
			if !isSameLoopbackRefererURL(trimmed, reqPort) {
				rejectForbidden(w, "forbidden cross-origin referer")
				return
			}
		}

		if r.Method == http.MethodOptions {
			if strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != "" && !hasOrigin {
				rejectForbidden(w, "forbidden cors preflight")
				return
			}
			stripCORSHeaders(w.Header())
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(&noCORSResponseWriter{ResponseWriter: w}, r)
	})
}

type noCORSResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *noCORSResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		stripCORSHeaders(w.Header())
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *noCORSResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		stripCORSHeaders(w.Header())
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *noCORSResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *noCORSResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func stripCORSHeaders(h http.Header) {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "access-control-allow-") {
			h.Del(k)
		}
	}
}

func rejectForbidden(w http.ResponseWriter, msg string) {
	stripCORSHeaders(w.Header())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

func isUnixDomainSocketPeer(remoteAddr string) bool {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" || trimmed == "@" || strings.HasPrefix(trimmed, "/") {
		return true
	}
	return false
}

func isAllowedUDSHost(rawHost string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(rawHost))
	if trimmed == "" {
		return true
	}
	if h, p, err := net.SplitHostPort(trimmed); err == nil {
		if !isValidPortNumber(p) {
			return false
		}
		trimmed = strings.ToLower(strings.TrimSpace(h))
	}
	switch trimmed {
	case "unix", "localhost", "127.0.0.1", "::1", "[::1]", "gorness", "daemon.sock":
		return true
	default:
		return false
	}
}

func parseLoopbackHostPort(rawHost string) (host, port string, ok bool) {
	trimmed := strings.TrimSpace(rawHost)
	if trimmed == "" {
		return "", "", false
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := trimmed[1 : len(trimmed)-1]
		if !isLoopbackHostname(inner) {
			return "", "", false
		}
		return normalizeLoopbackHost(inner), "80", true
	}

	if h, p, err := net.SplitHostPort(trimmed); err == nil {
		if !isLoopbackHostname(h) || !isValidPortNumber(p) {
			return "", "", false
		}
		return normalizeLoopbackHost(h), p, true
	}

	if !isLoopbackHostname(trimmed) {
		return "", "", false
	}
	return normalizeLoopbackHost(trimmed), "80", true
}

func isValidPortNumber(rawPort string) bool {
	if rawPort == "" {
		return false
	}
	n, err := strconv.Atoi(rawPort)
	return err == nil && n >= 1 && n <= 65535
}

func isSameLoopbackOriginHeader(rawOrigin, expectedPort string) bool {
	if strings.EqualFold(rawOrigin, "null") {
		return false
	}
	u, err := url.Parse(rawOrigin)
	if err != nil || u == nil {
		return false
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return matchLoopbackURLHostPort(u, expectedPort)
}

func isSameLoopbackRefererURL(rawURL, expectedPort string) bool {
	if strings.EqualFold(rawURL, "null") {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return false
	}
	return matchLoopbackURLHostPort(u, expectedPort)
}

func matchLoopbackURLHostPort(u *url.URL, expectedPort string) bool {
	if !strings.EqualFold(u.Scheme, "http") || u.User != nil || u.Host == "" {
		return false
	}
	hostname := u.Hostname()
	if !isLoopbackHostname(hostname) {
		return false
	}
	uPort := u.Port()
	if uPort == "" {
		uPort = "80"
	}
	wantPort := expectedPort
	if wantPort == "" {
		wantPort = "80"
	}
	return uPort == wantPort
}

func isLoopbackHostname(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func normalizeLoopbackHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
