package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LoopbackGuard 强制执行本机回环同源守卫与零 CORS 铁律：
// 1. 校验 Host 头防 DNS Rebinding（仅允许 127.0.0.1 / localhost / [::1]）；
// 2. 拦截 Sec-Fetch-Site: cross-site 与 Origin: null（沙箱 iframe 透明源）；
// 3. 校验非空 Origin 与 Referer 必须匹配当前服务的 http://<loopback>:<port>；
// 4. 全站响应严禁输出任何 Access-Control-Allow-* 跨域头。
func LoopbackGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripCORSHeaders(w.Header())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")

		_, reqPort, hostOK := parseLoopbackHostPort(r.Host)
		if !hostOK {
			rejectForbidden(w, "forbidden host header")
			return
		}

		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
			rejectForbidden(w, "forbidden cross-site request")
			return
		}

		for _, rawOrigin := range r.Header.Values("Origin") {
			trimmed := strings.TrimSpace(rawOrigin)
			if trimmed == "" {
				continue
			}
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

func parseLoopbackHostPort(rawHost string) (host, port string, ok bool) {
	trimmed := strings.TrimSpace(rawHost)
	if trimmed == "" {
		return "", "", false
	}

	if h, p, err := net.SplitHostPort(trimmed); err == nil {
		if !isLoopbackHostname(h) || p == "" {
			return "", "", false
		}
		return normalizeLoopbackHost(h), p, true
	}

	unbracketed := trimmed
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		unbracketed = trimmed[1 : len(trimmed)-1]
	}
	if !isLoopbackHostname(unbracketed) {
		return "", "", false
	}
	return normalizeLoopbackHost(unbracketed), "80", true
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
