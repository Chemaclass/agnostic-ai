package authority

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type originPermission struct{}

// ParseOrigin accepts the scheme, host, and optional port of a browser origin.
func ParseOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("origin must contain only an http or https scheme, host, and optional port")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("origin has an invalid port")
		}
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

func loopbackHost(hostPort string) bool {
	host := hostPort
	if parsed, port, err := net.SplitHostPort(hostPort); err == nil {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
		host = parsed
	} else if strings.Contains(hostPort, ":") {
		if !strings.HasPrefix(hostPort, "[") || !strings.HasSuffix(hostPort, "]") {
			return false
		}
		host = strings.Trim(hostPort, "[]")
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RequestAllowed shares the same local boundary across reads and edits.
func RequestAllowed(r *http.Request, allowedOrigins ...string) bool {
	if !loopbackHost(r.Host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return !strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site")
	}
	normalized, err := ParseOrigin(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	own, err := ParseOrigin(scheme + "://" + r.Host)
	if err == nil && normalized == own {
		return true
	}
	if trusted, _ := r.Context().Value(originPermission{}).(string); trusted == normalized {
		return true
	}
	for _, allowed := range allowedOrigins {
		if normalized == allowed {
			return true
		}
	}
	return false
}

// CORS rejects foreign hosts and origins before any route reads local data.
func CORS(next http.Handler, allowedOrigins ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusMisdirectedRequest, fmt.Errorf("request Host must name a loopback address"))
			return
		}
		if !RequestAllowed(r, allowedOrigins...) {
			writeError(w, http.StatusForbidden, fmt.Errorf("request host or origin is not allowed"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			normalized, _ := ParseOrigin(origin)
			r = r.WithContext(context.WithValue(r.Context(), originPermission{}, normalized))
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
