package authority

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ParseOrigin validates one --allow-origin value and returns it in the
// form a browser sends: a lower-case http or https scheme and host, an
// optional port, and nothing else.
func ParseOrigin(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	u, err := url.Parse(text)
	switch {
	case err != nil:
		return "", fmt.Errorf("origin %q: %w", raw, err)
	case u.Scheme != "http" && u.Scheme != "https":
		return "", fmt.Errorf("origin %q must start with http:// or https://", raw)
	case u.Host == "" || strings.HasSuffix(u.Host, ":"):
		return "", fmt.Errorf("origin %q names no host", raw)
	case u.User != nil:
		return "", fmt.Errorf("origin %q may not hold a user name or password", raw)
	case u.Path != "" && u.Path != "/", u.RawQuery != "", u.ForceQuery, u.Fragment != "", strings.Contains(text, "#"):
		return "", fmt.Errorf("origin %q may hold only a scheme, a host, and a port", raw)
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

// OriginGuard refuses a browser request from any origin other than the
// request's own or one of allowed, with a 403 and no CORS headers, even
// for a simple request a browser sends without asking first. An allowed
// origin gets CORS headers and its preflight answered. A request without
// an Origin header, such as one from curl, passes untouched.
func OriginGuard(next http.Handler, allowed []string) http.Handler {
	set := map[string]bool{}
	for _, o := range allowed {
		set[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Origin")
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		normal := strings.ToLower(origin)
		switch {
		case normal == scheme+"://"+strings.ToLower(r.Host):
		case set[normal]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		default:
			writeError(w, http.StatusForbidden, fmt.Errorf("origin %s may not call this API; start serve with --allow-origin %s to allow it", origin, origin))
			return
		}
		next.ServeHTTP(w, r)
	})
}
