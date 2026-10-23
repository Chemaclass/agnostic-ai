package authority

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestRequestAllowed_RequiresLoopbackHostAndExactOrigin(t *testing.T) {
	cases := []struct {
		name, host, origin, site string
		allowed                  []string
		want                     bool
	}{
		{name: "CLI", host: "127.0.0.1:4310", want: true},
		{name: "same origin", host: "localhost:4310", origin: "http://localhost:4310", want: true},
		{name: "IPv6", host: "[::1]:4310", origin: "http://[::1]:4310", want: true},
		{name: "foreign host", host: "attacker.example", want: false},
		{name: "foreign IP", host: "192.0.2.1:4310", want: false},
		{name: "malformed host", host: "localhost:bad", want: false},
		{name: "other local port", host: "127.0.0.1:4310", origin: "http://127.0.0.1:5173", want: false},
		{name: "other local name", host: "127.0.0.1:4310", origin: "http://localhost:4310", want: false},
		{name: "explicit origin", host: "127.0.0.1:4310", origin: "http://localhost:5173", allowed: []string{"http://localhost:5173"}, want: true},
		{name: "origin path", host: "127.0.0.1:4310", origin: "http://127.0.0.1:4310/", want: false},
		{name: "null origin", host: "127.0.0.1:4310", origin: "null", want: false},
		{name: "cross-site without origin", host: "127.0.0.1:4310", site: "cross-site", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/health", nil)
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.site)
			if got := RequestAllowed(req, tc.allowed...); got != tc.want {
				t.Errorf("allowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHandler_RejectsForeignHostsOnEveryRouteBeforeLoading(t *testing.T) {
	loads := 0
	h := NewHandler(func() (*config.Config, spec.Bundle, error) { loads++; return nil, spec.Bundle{}, nil }, HandlerOptions{})
	for _, path := range []string{"/", "/assets/app.js", pathView, pathSchema, pathHealth, pathImpact, "/api/missing"} {
		req := httptest.NewRequest(http.MethodGet, "http://attacker.example"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("%s status = %d", path, rec.Code)
		}
	}
	if loads != 0 {
		t.Errorf("loaded %d times for refused requests", loads)
	}
}
