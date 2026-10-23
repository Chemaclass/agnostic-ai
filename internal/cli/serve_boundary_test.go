package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestServeMux_RefusesForeignHostsOnAllRoutes(t *testing.T) {
	loads := 0
	h := newServeMux(func() (*config.Config, spec.Bundle, error) { loads++; return nil, spec.Bundle{}, nil }, authority.HandlerOptions{})
	for _, path := range []string{"/", "/api/health", "/api/views/authority", "/api/views/authority/schema", "/api/views/authority/impact", "/api/views/overview", editBuiltinPath, editValuePath} {
		req := httptest.NewRequest(http.MethodGet, "http://attacker.example"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("%s status = %d", path, rec.Code)
		}
	}
	if loads != 0 {
		t.Errorf("loaded %d times", loads)
	}
}

func TestServeMux_StalledBodyDoesNotHoldAPILock(t *testing.T) {
	h := newServeMux(func() (*config.Config, spec.Bundle, error) { return nil, spec.Bundle{}, nil }, authority.HandlerOptions{})
	reader, writer := io.Pipe()
	started := make(chan struct{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/views/authority/impact", &bodyReadNotice{Reader: reader, started: started})
	req.Header.Set("Content-Type", "application/json")
	finished := make(chan struct{})
	go func() { h.ServeHTTP(httptest.NewRecorder(), req); close(finished) }()
	<-started
	defer func() { _ = writer.Close(); <-finished }()
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/views/authority/schema", nil))
		done <- rec.Code
	}()
	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Errorf("schema status = %d", status)
		}
	case <-time.After(2 * time.Second):
		t.Error("stalled body held the API lock")
	}
}

func TestValidateServeAddr_RequiresExplicitLoopbackIP(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:4310", ":4310", "[::]:4310", "localhost:4310", "192.0.2.1:4310"} {
		if err := validateServeAddr(addr); err == nil {
			t.Errorf("accepted %s", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:4310", "[::1]:4310"} {
		if err := validateServeAddr(addr); err != nil {
			t.Errorf("refused %s: %v", addr, err)
		}
	}
}

func TestServedFindingMessage_RedactsQuotedRulesWithoutDroppingFindings(t *testing.T) {
	for _, message := range []string{`rule "shell(curl -H \"X-Api-Key: first-secret\")" is invalid`, `rule "shell(curl -H \"X-Api-Key: second-secret\")" is invalid`} {
		if shown := servedFindingMessage(message); strings.Contains(shown, "secret") {
			t.Errorf("finding retained credential: %q", shown)
		}
	}
}

func TestInstalledState_DoesNotResolveExternalAdapter(t *testing.T) {
	_, err := installedState("untrusted-external", &config.Config{}, spec.Bundle{}, t.TempDir())
	if err == nil || !strings.HasPrefix(err.Error(), "installed state for untrusted-external:") {
		t.Errorf("external state = %v", err)
	}
}

type bodyReadNotice struct {
	io.Reader
	started chan struct{}
	once    sync.Once
}

func (b *bodyReadNotice) Read(data []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	return b.Reader.Read(data)
}

func TestServeMux_UsesOneOriginPolicyForReadsWritesAndPreflights(t *testing.T) {
	h := newServeMux(func() (*config.Config, spec.Bundle, error) { return nil, spec.Bundle{}, nil }, authority.HandlerOptions{AllowedOrigins: []string{"http://localhost:5173"}})
	for _, path := range []string{"/", "/api/health", "/api/views/authority/schema", "/api/views/authority/impact", "/api/views/overview", editBuiltinPath, editValuePath} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
			req := httptest.NewRequest(method, "http://127.0.0.1:4310"+path, strings.NewReader("{}"))
			req.Header.Set("Origin", "http://127.0.0.1:5173")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s %s foreign origin = %d %v", method, path, rec.Code, rec.Header())
			}
			req = httptest.NewRequest(method, "http://127.0.0.1:4310"+path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s cross-site without Origin = %d", method, path, rec.Code)
			}
		}
	}
	for _, origin := range []string{"", "http://127.0.0.1:4310", "http://localhost:5173"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310/api/views/authority/schema", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("allowed origin %q status = %d", origin, rec.Code)
		}
	}
}
