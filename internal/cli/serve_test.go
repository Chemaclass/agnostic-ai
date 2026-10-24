package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestServe_RejectsAPIOnlyWithClientFlags(t *testing.T) {
	for _, flags := range [][]string{{"--api-only", "--open"}, {"--api-only", "--ui", "client"}} {
		cmd := newServeCmd()
		cmd.SetArgs(flags)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--api-only cannot be combined") {
			t.Errorf("%v: error = %v", flags, err)
		}
	}
}

func TestServeMux_ServesHealthAndTheView(t *testing.T) {
	silence(t)
	testutil.Chdir(t, demoProject(t))
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles, AllowedOrigins: []string{"http://localhost:5173"}}))
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
