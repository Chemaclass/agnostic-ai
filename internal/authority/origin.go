package authority

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
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
		return "", errors.New("origin must contain only an http or https scheme, host, and optional port")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("origin has an invalid port")
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
	return slices.Contains(allowedOrigins, normalized)
}

const deniedNavigation = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<title>Open agnostic-ai directly</title>
<h1>Open agnostic-ai directly</h1>
<p>Copy the address from the browser bar and paste it into a new tab to open the local interface.</p>
</html>`

// CORS rejects foreign hosts and origins before any route reads local data.
func CORS(next http.Handler, allowedOrigins ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusMisdirectedRequest, errors.New("request Host must name a loopback address"))
			return
		}
		if !RequestAllowed(r, allowedOrigins...) {
			path := r.URL.Path
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.Header.Get("Sec-Fetch-Mode") == "navigate" && path != "/api" && !strings.HasPrefix(path, "/api/") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(deniedNavigation))
				return
			}
			writeError(w, http.StatusForbidden, errors.New("request host or origin is not allowed"))
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
