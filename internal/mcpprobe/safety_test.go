package mcpprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func isolateCache(t *testing.T) {
	t.Helper()
	base := testutil.IsolateUserCache(t)
	path, err := CachePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("MCP cache %s escaped temporary base %s: %v", path, base, err)
	}
}

func TestCachePath_DirectoryAliasesShareTheListing(t *testing.T) {
	isolateCache(t)
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-link")
	testutil.DirectoryAlias(t, root, alias)
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	cache := Cache{Version: CacheVersion, Root: alias, Servers: map[string]ServerResult{"workspace": {Tools: []Tool{{Name: "workspace-tool"}}}}}
	if err := SaveCache(alias, cache); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(physical); len(got.Servers["workspace"].Tools) != 1 {
		t.Errorf("listing written through alias is missing through physical root: %+v", got)
	}
}

func TestListTools_HTTPRedactsCredentialsEchoedByServer(t *testing.T) {
	secret := "synthetic-secret-with-spaces here"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer error_description="rejected `+secret+`"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer " + secret}}, "test")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("raw probe error exposes server's credential echo: %v", err)
	}
}

func TestListTools_RedactsToolFieldsBeforeReturning(t *testing.T) {
	secret := "synthetic-private-tool-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.ID == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result := any(map[string]any{})
		if request.Method == "tools/list" {
			result = listResult{Tools: []Tool{{Name: "safe_name", Title: secret, Description: "Echo " + secret}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer srv.Close()
	tools, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL, SensitiveValues: []string{secret}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Errorf("raw tools expose the resolved value: %s", data)
	}
}

func TestListTools_PaginationLimitDoesNotClaimPartialListingIsComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.ID == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result := any(map[string]any{})
		if request.Method == "tools/list" {
			result = listResult{Tools: []Tool{{Name: "partial"}}, NextCursor: "again"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer srv.Close()
	tools, err := ListTools(ctx(t), Server{Type: "http", URL: srv.URL}, "test")
	if err == nil || len(tools) != 0 {
		t.Errorf("partial listing reported as complete: tools=%+v, error=%v", tools, err)
	}
}

func TestLoadCache_IgnoresListingsFromBeforeCredentialRedaction(t *testing.T) {
	root := t.TempDir()
	isolateCache(t)
	path, err := CachePath(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"servers":{"old":{"error":"unredacted-secret"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(root); len(got.Servers) != 0 {
		t.Errorf("old cache remains visible: %+v", got)
	}
}

func TestHeaders_AlreadyCanceledContextDoesNotLaunchHelper(t *testing.T) {
	c, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Headers(c, "unused", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("helper failure = %v", err)
	}
}

func TestHeaders_CancellationStopsRecordedDescendant(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := "'" + strings.ReplaceAll(exe, "'", "'\"'\"'") + "'"
	if runtime.GOOS == "windows" {
		helper = `"` + exe + `"`
	}
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	t.Setenv("MCPPROBE_FAKE_SERVER", "descendant")
	t.Setenv("MCPPROBE_CHILD_PID", pidFile)
	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := Headers(c, helper, root); done <- err }()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid != 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("fake helper did not record its child PID")
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Kill(); _ = child.Release() }()
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("helper error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled helper did not return")
	}
	deadline = time.Now().Add(time.Second)
	for processRunning(t, pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(t, pid) {
		t.Errorf("owned helper descendant %d is still running", pid)
	}
}

func TestSaveCache_ReplacesPrivatelyAndKeepsOtherFiles(t *testing.T) {
	isolateCache(t)
	root := t.TempDir()
	cache := Cache{Version: CacheVersion, Root: root, Servers: map[string]ServerResult{"first": {Tools: []Tool{{Name: "first"}}}}}
	if err := SaveCache(root, cache); err != nil {
		t.Fatal(err)
	}
	path, err := CachePath(root)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(path), "other.json")
	if err := os.WriteFile(other, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache.Servers = map[string]ServerResult{"second": {Error: "not listed"}}
	if err := SaveCache(root, cache); err != nil {
		t.Fatal(err)
	}
	got := LoadCache(root)
	if len(got.Servers) != 1 || got.Servers["second"].Error != "not listed" {
		t.Errorf("replacement = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("cache permissions = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(other)
	if err != nil || string(data) != "untouched" {
		t.Errorf("other cache changed: %q, %v", data, err)
	}
}
