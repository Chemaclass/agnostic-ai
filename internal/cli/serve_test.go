package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestServeMux_ServesHealthAndTheView(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, nil))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	var health struct {
		OK       bool   `json:"ok"`
		Revision string `json:"revision"`
	}
	err = json.NewDecoder(resp.Body).Decode(&health)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || !health.OK || len(health.Revision) != 12 {
		t.Fatalf("health = %d %+v (%v)", resp.StatusCode, health, err)
	}

	resp, err = http.Get(srv.URL + "/api/views/authority?agentId=reviewer&target=claude")
	if err != nil {
		t.Fatal(err)
	}
	var v authority.View
	err = json.NewDecoder(resp.Body).Decode(&v)
	_ = resp.Body.Close()
	if err != nil || v.Agent.ID != "reviewer" || v.Snapshot.Revision != health.Revision {
		t.Errorf("view = %s at %s, want reviewer at %s (%v)", v.Agent.ID, v.Snapshot.Revision, health.Revision, err)
	}

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/views/overview", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	pre, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = pre.Body.Close()
	if pre.StatusCode != http.StatusForbidden || pre.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("preflight from another port = %d, allow-origin %q; want 403 and none", pre.StatusCode, pre.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestServeMux_AllowOriginOpensExactlyThatOrigin(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, []string{"http://localhost:5173"}))
	defer srv.Close()

	for origin, want := range map[string]int{"http://localhost:5173": http.StatusNoContent, "http://localhost:3000": http.StatusForbidden} {
		for _, path := range []string{"/api/views/overview", "/api/views/authority/impact"} {
			req, _ := http.NewRequest(http.MethodOptions, srv.URL+path, nil)
			req.Header.Set("Origin", origin)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			allowed := resp.Header.Get("Access-Control-Allow-Origin")
			if resp.StatusCode != want || want == http.StatusNoContent && allowed != origin || want != http.StatusNoContent && allowed != "" {
				t.Errorf("preflight %s from %s = %d, allow-origin %q", path, origin, resp.StatusCode, allowed)
			}
		}
	}
}

func TestServeMux_AStalledBodyDoesNotHoldTheLock(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, nil))
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	stalled := "POST /api/views/authority/impact HTTP/1.1\r\nHost: " + srv.Listener.Addr().String() + "\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{\"changes\":"
	if _, err := conn.Write([]byte(stalled)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("health while a body stalls: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health = %d", resp.StatusCode)
	}
}

func TestServe_RefusesABadOriginOrANonLoopbackAddressBeforeListening(t *testing.T) {
	silence(t)
	for _, args := range [][]string{
		{"serve", "--allow-origin", "http://localhost:5173/app"},
		{"serve", "--allow-origin", "*"},
		{"serve", "--addr", "0.0.0.0:0"},
		{"serve", "--addr", ":0"},
		{"serve", "--addr", "192.168.1.10:0"},
		{"serve", "--addr", "example.com:0"},
	} {
		_, err := runCLI(t, args...)
		flag := args[1]
		if err == nil || !strings.Contains(err.Error(), flag) || strings.Contains(err.Error(), "listen on") {
			t.Errorf("%v: err = %v, want a refusal that names %s", args, err, flag)
		}
	}
}

func TestServe_RejectsARequestAddressedToAnotherHost(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	listen := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4310}
	h := requireLocalHost(ok, listen)
	for host, want := range map[string]int{
		"127.0.0.1:4310":    http.StatusOK,
		"localhost:4310":    http.StatusOK,
		"[::1]:4310":        http.StatusOK,
		"LOCALHOST:4310":    http.StatusOK,
		"":                  http.StatusMisdirectedRequest,
		"evil.example:4310": http.StatusMisdirectedRequest,
		"127.0.0.1.nip.io":  http.StatusMisdirectedRequest,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/views/authority", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %s = %d, want %d", host, rec.Code, want)
		}
		if want != http.StatusOK && (!strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || !strings.Contains(rec.Body.String(), `"error"`)) {
			t.Errorf("Host %s answered %q, want a JSON error", host, rec.Body)
		}
	}
}

func TestServe_AProxiedClientThatKeepsItsHostIsSameOrigin(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	listen := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4310}
	h := requireLocalHost(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, nil), listen)
	for host, want := range map[string]int{"127.0.0.1:5173": http.StatusOK, "127.0.0.1:4310": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodPost, "/api/views/authority/impact", strings.NewReader(`{"changes":[]}`))
		req.Host = host
		req.Header.Set("Origin", "http://127.0.0.1:5173")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %s with the dev client's Origin = %d, want %d: %s", host, rec.Code, want, rec.Body)
		}
	}
}
