package authority

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseOrigin_AcceptsSchemeHostAndPortOnly(t *testing.T) {
	for raw, want := range map[string]string{
		"http://localhost:5173":   "http://localhost:5173",
		"HTTP://LocalHost:5173/":  "http://localhost:5173",
		"https://tools.example":   "https://tools.example",
		"http://[::1]:8080":       "http://[::1]:8080",
		" http://127.0.0.1:4173 ": "http://127.0.0.1:4173",
	} {
		got, err := ParseOrigin(raw)
		if err != nil || got != want {
			t.Errorf("ParseOrigin(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", "null", "*", "localhost:5173", "ftp://localhost", "http://", "http://localhost:",
		"http://localhost:5173/app", "http://localhost:5173?x=1", "http://localhost:5173#top",
		"http://user:pw@localhost:5173", "file:///tmp/index.html",
	} {
		if got, err := ParseOrigin(raw); err == nil {
			t.Errorf("ParseOrigin(%q) = %q, want an error", raw, got)
		}
	}
}

// guarded serves req through OriginGuard and reports whether the next
// handler ran.
func guarded(req *http.Request, allowed ...string) (*httptest.ResponseRecorder, bool) {
	ran := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	OriginGuard(next, allowed).ServeHTTP(rec, req)
	return rec, ran
}

func originRequest(method, host, origin string) *http.Request {
	req := httptest.NewRequest(method, "/api/views/authority/impact", strings.NewReader("{}"))
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestOriginGuard_RequestsWithoutOriginPass(t *testing.T) {
	rec, ran := guarded(originRequest(http.MethodPost, "127.0.0.1:4310", ""))
	if !ran || rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("status %d, ran %v, headers %v", rec.Code, ran, rec.Header())
	}
}

func TestOriginGuard_SameOriginRequestsPassWithoutCORSHeaders(t *testing.T) {
	for host, origin := range map[string]string{
		"127.0.0.1:5173": "http://127.0.0.1:5173",
		"LOCALHOST:4310": "http://localhost:4310",
	} {
		rec, ran := guarded(originRequest(http.MethodPost, host, origin))
		if !ran || rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s from %s: status %d, ran %v, headers %v", host, origin, rec.Code, ran, rec.Header())
		}
	}
}

func TestOriginGuard_ForeignOriginsAreRefusedEvenForSimpleRequests(t *testing.T) {
	for _, origin := range []string{"http://localhost:3000", "http://127.0.0.1:4173", "https://evil.example", "null"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
			req := originRequest(method, "127.0.0.1:4310", origin)
			req.Header.Set("Content-Type", "text/plain")
			rec, ran := guarded(req, "http://localhost:5173")
			if ran || rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s %s: status %d, ran %v, headers %v", method, origin, rec.Code, ran, rec.Header())
			}
			if !strings.Contains(rec.Body.String(), "--allow-origin") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Errorf("%s %s: body %s", method, origin, rec.Body)
			}
		}
	}
}

func TestOriginGuard_AllowedOriginGetsCORSAndThePreflight(t *testing.T) {
	const origin = "http://localhost:5173"
	rec, ran := guarded(originRequest(http.MethodPost, "127.0.0.1:4310", origin), origin)
	if !ran || rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != origin || rec.Header().Get("Vary") != "Origin" {
		t.Errorf("request: status %d, ran %v, headers %v", rec.Code, ran, rec.Header())
	}
	pre, ran := guarded(originRequest(http.MethodOptions, "127.0.0.1:4310", origin), origin)
	if ran || pre.Code != http.StatusNoContent || pre.Header().Get("Access-Control-Allow-Origin") != origin || !strings.Contains(pre.Header().Get("Access-Control-Allow-Methods"), "POST") || pre.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Errorf("preflight: status %d, ran %v, headers %v", pre.Code, ran, pre.Header())
	}
}
