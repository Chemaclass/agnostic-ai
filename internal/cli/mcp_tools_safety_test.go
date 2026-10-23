package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func isolateMCPToolsCache(t *testing.T) string {
	t.Helper()
	base := testutil.IsolateUserCache(t)
	path, err := mcpprobe.CachePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("MCP cache %s escaped temporary base %s: %v", path, base, err)
	}
	return base
}

func TestMCPServer_ResolvesWorkingDirectoryFromSelectedRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MCP_TEST_SUBDIR", "nested")
	t.Setenv("workspaceFolder", "ignored-environment-value")
	for _, cwd := range []string{"", "nested", "${workspaceFolder}/nested", "${MCP_TEST_SUBDIR}", "nested/../nested", "${workspaceFolder}/nested/../nested", filepath.ToSlash(root) + "/nested"} {
		t.Run(cwd, func(t *testing.T) {
			server, err := mcpServer(spec.Entry{Meta: map[string]any{"command": "unused", "cwd": cwd}}, root)
			if err != nil {
				t.Fatal(err)
			}
			want := root
			if cwd != "" {
				want = filepath.Join(root, "nested")
			}
			if server.Cwd != want {
				t.Errorf("cwd = %q, want %q", server.Cwd, want)
			}
		})
	}
}

func TestMCPTools_OverviewFindsListingThroughProjectDirectoryAlias(t *testing.T) {
	silence(t)
	testutil.Chdir(t, t.TempDir())
	isolateMCPToolsCache(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	project := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-link")
	testutil.DirectoryAlias(t, project, alias)
	srv := fakeMCP(t, "synthetic-credential")
	mustWriteGlobalTest(t, filepath.Join(project, "agnostic-ai.yaml"), "targets: [claude]\nbuiltins: []\n")
	mustWriteGlobalTest(t, filepath.Join(project, ".agnostic-ai", "mcps", "workspace.yaml"), "type: http\nurl: "+srv.URL+"\nheaders:\n  Authorization: Bearer synthetic-credential\n")
	root, entries, err := mcpToolsScope(alias, false)
	if err != nil {
		t.Fatal(err)
	}
	cache := listMCPTools(context.Background(), root, entries, time.Second, "test")
	if err := mcpprobe.SaveCache(root, cache); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, physical)
	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range overview.Build(in).Sections {
		for _, item := range section.Items {
			if item.Name != "workspace" {
				continue
			}
			if item.Tools == nil || item.Tools.State != overview.ToolsListed || len(item.Tools.Tools) != 1 || item.Tools.Tools[0].Name != "send_email" {
				t.Errorf("overview lost alias listing: %+v", item.Tools)
			}
			return
		}
	}
	t.Fatal("overview omitted configured workspace MCP")
}

func TestMCPServer_PreservesLiteralEnvironmentValues(t *testing.T) {
	t.Setenv("MCP_LITERAL_TOKEN", "expanded")
	entry := spec.Entry{Meta: map[string]any{"command": "unused", "env": map[string]any{"TOKEN": "${MCP_LITERAL_TOKEN}"}}, Literals: map[string]map[string]bool{"env": {"TOKEN": true}}}
	server, err := mcpServer(entry, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if server.Env["TOKEN"] != "${MCP_LITERAL_TOKEN}" {
		t.Errorf("literal value expanded: %q", server.Env["TOKEN"])
	}
}

func TestMCPToolsProcess(t *testing.T) {
	switch os.Getenv("MCP_TOOLS_TEST_PROCESS") {
	case "helper":
		time.Sleep(time.Second)
		_, _ = os.Stdout.WriteString("{}")
		return
	case "server":
		if _, err := os.Stat("project-marker"); err != nil {
			t.Fatal(err)
		}
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(in.Bytes(), &req) != nil || req.ID == 0 {
				continue
			}
			result := map[string]any{}
			if req.Method == "tools/list" {
				result["tools"] = []map[string]string{{"name": "project-tool"}}
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
		return
	}
}

func TestMCPTools_ChildUsesSelectedRootFromAnotherDirectory(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, root)
	}
	mustWriteGlobalTest(t, filepath.Join(root, "project-marker"), "project")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	entry := spec.Entry{Name: "project", Meta: map[string]any{"command": exe, "args": []any{"-test.run=^TestMCPToolsProcess$"}, "env": map[string]any{"MCP_TOOLS_TEST_PROCESS": "server"}}}
	cache := listMCPTools(context.Background(), root, []spec.Entry{entry}, time.Second, "test")
	result := cache.Servers[entry.Name]
	if result.Error != "" || len(result.Tools) != 1 || result.Tools[0].Name != "project-tool" {
		t.Errorf("selected project listing = %+v", result)
	}
}

func TestMCPTools_HeaderHelperSharesTimeoutAndParentCancellation(t *testing.T) {
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, t.TempDir())
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := "'" + strings.ReplaceAll(exe, "'", "'\"'\"'") + "' -test.run=^TestMCPToolsProcess$"
	if runtime.GOOS == "windows" {
		helper = `"` + exe + `" -test.run=^TestMCPToolsProcess$`
	}
	t.Setenv("MCP_TOOLS_TEST_PROCESS", "helper")
	entry := spec.Entry{Name: "helper", Meta: map[string]any{"command": "unused", "headersHelper": helper}}
	for _, parentCancel := range []bool{false, true} {
		parent, cancel := context.WithCancel(context.Background())
		timeout := 40 * time.Millisecond
		if parentCancel {
			timeout = time.Minute
			time.AfterFunc(40*time.Millisecond, cancel)
		}
		start := time.Now()
		result := listMCPTools(parent, t.TempDir(), []spec.Entry{entry}, timeout, "test").Servers[entry.Name]
		cancel()
		if result.Error == "" || !strings.Contains(result.Error, "context") {
			t.Errorf("helper failure = %q", result.Error)
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("helper cancellation took %s", elapsed)
		}
	}
}

func TestMCPTools_BoundsActiveProbesAndSkipsCanceledQueue(t *testing.T) {
	var active, peak, calls atomic.Int32
	parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		if n == mcpProbeWorkers {
			cancel()
		}
		<-parent.Done()
	}))
	defer srv.Close()
	entries := make([]spec.Entry, mcpProbeWorkers*3)
	for i := range entries {
		entries[i] = spec.Entry{Name: string(rune('a' + i)), Meta: map[string]any{"type": "http", "url": srv.URL}}
	}
	cache := listMCPTools(parent, t.TempDir(), entries, time.Second, "test")
	if peak.Load() != mcpProbeWorkers || calls.Load() > mcpProbeWorkers {
		t.Errorf("active=%d, started=%d, limit=%d", peak.Load(), calls.Load(), mcpProbeWorkers)
	}
	if len(cache.Servers) != len(entries) {
		t.Errorf("cache contains %d servers, want %d", len(cache.Servers), len(entries))
	}
	for name, result := range cache.Servers {
		if result.Error == "" {
			t.Errorf("canceled %s recorded as successful", name)
		}
	}
}

func TestMCPServer_DisabledAndUnsupportedDoNotLaunchHelpers(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "helper-was-started")
	helper := "echo started > '" + marker + "'"
	if runtime.GOOS == "windows" {
		helper = `echo started > "` + marker + `"`
	}
	for _, meta := range []map[string]any{{"disabled": true, "headersHelper": helper}, {"type": "sse", "headersHelper": helper}, {"type": "ws", "headersHelper": helper}} {
		if _, err := mcpServer(spec.Entry{Meta: meta}, root); err == nil {
			t.Errorf("unavailable transport accepted: %+v", meta)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("disabled or unsupported server launched helper: %v", err)
	}
}

func TestMCPTools_RedactsResolvedCredentialsBeforeCachingOrDisplaying(t *testing.T) {
	root := t.TempDir()
	isolateMCPToolsCache(t)
	t.Setenv("MCP_SYNTHETIC_TOKEN", "synthetic-query-secret-123")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()
	url := strings.Replace(srv.URL, "://", "://synthetic-user:synthetic-pass@", 1) + "/?api_key=${MCP_SYNTHETIC_TOKEN}"
	entry := spec.Entry{Name: "private", Kind: spec.KindMCP, Meta: map[string]any{"type": "http", "url": url}}
	cache := listMCPTools(context.Background(), root, []spec.Entry{entry}, time.Second, "test")
	if cache.Servers[entry.Name].Error == "" {
		t.Fatal("broken fake HTTP connection did not produce an error")
	}
	if err := mcpprobe.SaveCache(root, cache); err != nil {
		t.Fatal(err)
	}
	path, err := mcpprobe.CachePath(root)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := json.Marshal(overview.Build(overview.Inputs{Bundle: spec.Bundle{MCPs: []spec.Entry{entry}}, MCPTools: cache}))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", root)
	mustWriteGlobalTest(t, filepath.Join(root, "agnostic-ai.yaml"), "targets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(root, "mcps", "private.yaml"), "type: http\nurl: "+url+"\n")
	text, err := runCLI(t, "mcp", "tools", "--global", "--timeout", "1s")
	if err != nil {
		t.Fatal(err)
	}
	structured, err := runCLI(t, "mcp", "tools", "--global", "--timeout", "1s", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{string(disk), string(view), text, structured} {
		for _, secret := range []string{"synthetic-user", "synthetic-pass", "synthetic-query-secret-123"} {
			if strings.Contains(output, secret) {
				t.Errorf("cached or served output contains %q", secret)
			}
		}
	}
}
