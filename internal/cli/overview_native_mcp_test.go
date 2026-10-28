package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// nativeMCPSecrets are the synthetic credentials the fixtures hide in
// every value field a server can carry.
var nativeMCPSecrets = []string{"ghp_synthetic123", "synthetic-bearer", "synthetic-pass", "synthetic-q", "synthetic-arg"}

// nativeMCPHome makes an empty user home with the global source under
// it, so no file of the real home is read, and returns the home.
func nativeMCPHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "COPILOT_HOME", "GEMINI_CLI_HOME", "QWEN_HOME", "VIBE_HOME", "GROK_HOME"} {
		t.Setenv(name, "")
	}
	return home
}

// writeNativeMCPProject writes a claude, cursor, and codex project
// with a github spec, and tool files that hold servers with a
// credential in each value field: env, headers, URL, and args.
func writeNativeMCPProject(t *testing.T, home, dir string) {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor, codex]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "mcps", "github.yaml"), "command: github-mcp\n")
	claude, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"notion": map[string]any{"type": "http", "url": "https://u:synthetic-pass@mcp.notion.com/mcp?api_key=synthetic-q", "headers": map[string]any{"Authorization": "Bearer synthetic-bearer"}},
			"GitHub": map[string]any{"command": "gh", "env": map[string]any{"GITHUB_TOKEN": "ghp_synthetic123"}},
		},
		"projects": map[string]any{
			resolved:      map[string]any{"mcpServers": map[string]any{"local-db": map[string]any{"command": "db", "args": []any{"--token", "synthetic-arg"}, "env": map[string]any{"GITHUB_TOKEN": "ghp_synthetic123"}}}},
			"/other/root": map[string]any{"mcpServers": map[string]any{"elsewhere": map[string]any{"command": "x"}}},
		},
		"oauthAccount": map[string]any{"emailAddress": "someone@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), claude, 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": {
		"github": {"command": "github-mcp", "env": {"GITHUB_TOKEN": "ghp_synthetic123"}},
		"sentry": {"type": "sse", "url": "https://sentry.example/sse?token=synthetic-q", "headers": {"Authorization": "Bearer synthetic-bearer"}}
	}}`)
	mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"linear": {"url": "https://u:synthetic-pass@linear.example/mcp", "headers": {"Authorization": "Bearer synthetic-bearer"}}}}`)
	mustWrite(t, filepath.Join(dir, ".codex", "config.toml"), "[mcp_servers.docs]\ncommand = \"docs\"\nargs = [\"--token\", \"synthetic-arg\"]\nenv = { TOKEN = \"ghp_synthetic123\" }\n")
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.broken\ncommand = \"x\"\n")
}

func TestNativeMCPServers_ReadsEachTargetsProjectAndUserFiles(t *testing.T) {
	home := nativeMCPHome(t)
	dir := testutil.TempCwd(t)
	writeNativeMCPProject(t, home, dir)
	cfg := &config.Config{Targets: []string{"claude", "cursor", "codex"}}

	got := nativeMCPServers(home, ".", "", cfg, false)
	want := []overview.NativeMCP{
		{Name: "github", Target: "claude", Path: ".mcp.json", Scope: "project", Transport: "stdio", Import: "agnostic-ai import claude"},
		{Name: "sentry", Target: "claude", Path: ".mcp.json", Scope: "project", Transport: "sse", Import: "agnostic-ai import claude"},
		{Name: "docs", Target: "codex", Path: ".codex/config.toml", Scope: "project", Transport: "stdio", Import: "agnostic-ai import codex"},
		{Name: "local-db", Target: "claude", Path: "~/.claude.json", Scope: "project", Transport: "stdio"},
		{Name: "GitHub", Target: "claude", Path: "~/.claude.json", Scope: "user", Transport: "stdio", Import: "agnostic-ai import --global claude"},
		{Name: "notion", Target: "claude", Path: "~/.claude.json", Scope: "user", Transport: "http", Import: "agnostic-ai import --global claude"},
		{Name: "linear", Target: "cursor", Path: "~/.cursor/mcp.json", Scope: "user", Transport: "remote", Import: "agnostic-ai import --global cursor"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("servers =\n%+v\nwant\n%+v", got, want)
	}

	global := nativeMCPServers(home, ".", "", cfg, true)
	for _, s := range global {
		if s.Scope != overview.NativeScopeUser {
			t.Errorf("global page lists a %s server: %+v", s.Scope, s)
		}
	}
	if len(global) != 3 {
		t.Errorf("global servers = %+v, want the three user servers", global)
	}
}

func TestNativeMCPServers_LeavesOutWhatGlobalSyncWrote(t *testing.T) {
	home := nativeMCPHome(t)
	testutil.TempCwd(t)
	source := filepath.Join(home, "source")
	mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"db": {"command": "db"}, "linear": {"url": "https://linear.example/mcp"}}}`)
	mustWrite(t, filepath.Join(source, "state", "global.json"), `{"version": 1, "files": [], "mcp": {"cursor": {"db": {"command": "db"}}}}`)

	got := nativeMCPServers(home, ".", source, &config.Config{Targets: []string{"cursor"}}, true)
	if len(got) != 1 || got[0].Name != "linear" {
		t.Errorf("servers = %+v, want linear only", got)
	}
}

func TestNativeMCPServers_MissingOrBrokenFilesGiveNoServers(t *testing.T) {
	home := nativeMCPHome(t)
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Targets: []string{"claude", "codex", "vibe", "opencode"}}
	if got := nativeMCPServers(home, ".", "", cfg, false); got != nil {
		t.Errorf("no files: servers = %+v", got)
	}
	mustWrite(t, filepath.Join(home, ".claude.json"), "{not json")
	mustWrite(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": ["a list"]}`)
	mustWrite(t, filepath.Join(dir, ".vibe", "config.toml"), "mcp_servers = \"text\"\n")
	mustWrite(t, filepath.Join(dir, "opencode.json"), "[")
	if got := nativeMCPServers(home, ".", "", cfg, false); got != nil {
		t.Errorf("broken files: servers = %+v", got)
	}
}

func TestNativeMCPServers_ReadsTOMLArraysAndOpencode(t *testing.T) {
	home := nativeMCPHome(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, ".vibe", "config.toml"), "[[mcp_servers]]\nname = \"docs\"\ntransport = \"streamable-http\"\nurl = \"https://docs.example/mcp?key=synthetic-q\"\n")
	mustWrite(t, filepath.Join(dir, "opencode.json"), `{
		// comment
		"mcp": {"fs": {"type": "local", "command": ["fs"]}, "web": {"type": "remote", "url": "https://web.example"}}
	}`)
	got := nativeMCPServers(home, ".", "", &config.Config{Targets: []string{"vibe", "opencode"}}, false)
	want := []overview.NativeMCP{
		{Name: "docs", Target: "vibe", Path: ".vibe/config.toml", Scope: "project", Transport: "streamable-http", Import: "agnostic-ai import vibe"},
		{Name: "fs", Target: "opencode", Path: "opencode.json", Scope: "project", Transport: "stdio", Import: "agnostic-ai import opencode"},
		{Name: "web", Target: "opencode", Path: "opencode.json", Scope: "project", Transport: "remote", Import: "agnostic-ai import opencode"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("servers =\n%+v\nwant\n%+v", got, want)
	}
}

func TestNativeMCPTransport_ReturnsOnlyKnownNames(t *testing.T) {
	cases := []struct {
		server map[string]any
		want   string
	}{
		{map[string]any{"command": "x", "type": "http"}, "stdio"},
		{map[string]any{"type": "HTTP", "url": "u"}, "http"},
		{map[string]any{"type": "synthetic-bearer", "url": "u"}, "remote"},
		{map[string]any{"transport": "sse"}, "sse"},
		{map[string]any{"httpUrl": "u"}, "remote"},
		{map[string]any{"type": "ghp_synthetic123"}, ""},
	}
	for _, c := range cases {
		if got := nativeMCPTransport(c.server); got != c.want {
			t.Errorf("transport(%v) = %q, want %q", c.server, got, c.want)
		}
	}
}

// Every target whose adapter declares a project MCP file has a reader
// entry, so a new target cannot be missed silently.
func TestProjectNativeMCP_CoversEveryTargetWithAProjectMCPFile(t *testing.T) {
	cfg := &config.Config{}
	for _, name := range adapters.Names() {
		for _, a := range adapters.NativeArtifactsFor(name, cfg) {
			if a.Label != "MCP servers" {
				continue
			}
			if _, ok := projectNativeMCP[name]; !ok {
				t.Errorf("%s declares project MCP file %s but projectNativeMCP has no entry", name, a.Location)
			}
		}
	}
}

func TestOverviewNativeMCPs_KeepsEveryValueOutOfTheView(t *testing.T) {
	home := nativeMCPHome(t)
	dir := testutil.TempCwd(t)
	writeNativeMCPProject(t, home, dir)
	claudeJSON := filepath.Join(home, ".claude.json")
	before, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatal(err)
	}

	in, err := overviewInputsWith(".", false)
	if err != nil {
		t.Fatal(err)
	}
	built, err := json.Marshal(overview.Build(in))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	newOverviewHandler(".", false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+overview.OverviewPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	printed, err := runCLI(t, "view", "--json")
	if err != nil {
		t.Fatalf("view --json: %v\n%s", err, printed)
	}

	for name, output := range map[string]string{"built": string(built), "served": rec.Body.String(), "printed": printed} {
		var view overview.View
		if err := json.Unmarshal([]byte(output), &view); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		native := nativeRows(view)
		var names []string
		for _, n := range native {
			names = append(names, n.Name)
		}
		if strings.Join(names, ",") != "docs,linear,local-db,notion,sentry" {
			t.Errorf("%s: native rows = %v", name, names)
		}
		for _, secret := range append(nativeMCPSecrets, "someone@example.com", "mcp.notion.com") {
			if strings.Contains(output, secret) {
				t.Errorf("%s output contains %q", name, secret)
			}
		}
		raw, err := json.Marshal(native)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{`"command"`, `"args"`, `"url"`, `"env"`, `"headers"`} {
			if bytes.Contains(raw, []byte(key)) {
				t.Errorf("%s: native rows carry %s: %s", name, key, raw)
			}
		}
	}

	after, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(claudeJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || info.Mode().Perm() != 0o600 {
		t.Errorf("~/.claude.json changed: mode %v", info.Mode().Perm())
	}
}

func TestGlobalOverviewNativeMCPs_ListsUserServersOnly(t *testing.T) {
	home := nativeMCPHome(t)
	dir := testutil.TempCwd(t)
	writeNativeMCPProject(t, home, dir)
	mustWriteGlobalTest(t, filepath.Join(home, "source", "agnostic-ai.yaml"), "targets: [claude, cursor]\n")

	out, err := runCLI(t, "view", "--global", "--json")
	if err != nil {
		t.Fatalf("view --global: %v\n%s", err, out)
	}
	var view overview.View
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range nativeRows(view) {
		for _, h := range n.Holders {
			got = append(got, n.Name+":"+h.Scope+":"+h.Path)
		}
		if !strings.HasPrefix(n.Import, "agnostic-ai import --global ") {
			t.Errorf("%s import = %q", n.Name, n.Import)
		}
	}
	want := "GitHub:user:~/.claude.json,linear:user:~/.cursor/mcp.json,notion:user:~/.claude.json"
	if strings.Join(got, ",") != want {
		t.Errorf("global native rows = %v, want %s", got, want)
	}
	for _, secret := range nativeMCPSecrets {
		if strings.Contains(out, secret) {
			t.Errorf("global view contains %q", secret)
		}
	}
}

func nativeRows(v overview.View) []overview.NativeServer {
	for _, s := range v.Sections {
		if s.ID == overview.SectionTools {
			return s.Native
		}
	}
	return nil
}
