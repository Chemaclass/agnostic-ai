package cli

import (
	"context"
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
		OK   bool   `json:"ok"`
		Root string `json:"root"`
	}
	err = json.NewDecoder(resp.Body).Decode(&health)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || !health.OK || health.Root == "" {
		t.Fatalf("health = %d %+v (%v)", resp.StatusCode, health, err)
	}

	resp, err = http.Get(srv.URL + "/api/views/authority?agentId=reviewer&target=claude")
	if err != nil {
		t.Fatal(err)
	}
	var v authority.View
	err = json.NewDecoder(resp.Body).Decode(&v)
	_ = resp.Body.Close()
	if err != nil || v.Agent.ID != "reviewer" || len(v.Snapshot.Revision) != 12 {
		t.Errorf("view = %s at %s, want reviewer (%v)", v.Agent.ID, v.Snapshot.Revision, err)
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

func TestServeUntilInterrupted_StopsWithAnOpenConnectionThatSentNoRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveUntilInterrupted(ctx, ln, http.NotFoundHandler()) }()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if resp, err := http.Get("http://" + ln.Addr().String()); err == nil {
		_ = resp.Body.Close()
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve still waits for a connection that sent no request")
	}
}
