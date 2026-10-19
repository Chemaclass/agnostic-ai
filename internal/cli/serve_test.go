package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

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
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}))
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
	if pre.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Error("the mux does not answer a local preflight for routes mounted beside the authority view")
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
	}
}
