package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runImportGlobalTest(args ...string) (string, string, error) {
	var out, warnings bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&warnings)
	cmd.SetArgs(append([]string{"import", "--global"}, args...))
	err := cmd.Execute()
	return out.String(), warnings.String(), err
}

func TestImportGlobal_CodexConfigRoundTrips(t *testing.T) {
	home, source := globalAgentTestHome(t)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	mustWriteGlobalTest(t, codexPath, "model = \"gpt-6-luna\"\nmodel_reasoning_effort = \"high\"\n\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\nargs = [\"--stdio\"]\n")

	if _, warnings, err := runImportGlobalTest("codex"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	settings := readGlobalTest(t, filepath.Join(source, "settings", "imported.yaml"))
	if !strings.Contains(settings, "codex: gpt-6-luna") || !strings.Contains(settings, "codex: high") {
		t.Errorf("settings spec:\n%s", settings)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "docs.yaml")); err != nil {
		t.Fatalf("docs MCP spec: %v", err)
	}
	before := readGlobalTest(t, codexPath)
	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, codexPath); got != before {
		t.Errorf("sync after import changed config.toml:\n%s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "codex", "--check"); err != nil {
		t.Fatalf("check: %v", err)
	}
}

// Every JSON user file sync writes reads back through import into specs
// that render to the same bytes.
func TestImportGlobal_RoundTripsWhatSyncWrote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	targets := "claude,codex,copilot,cursor,gemini,qoder"
	settings := filepath.Join(source, "settings", "d.yaml")
	mustWriteGlobalTest(t, settings, `model:
  claude: opus
  codex: gpt-6-luna
  copilot: gpt-6-sol
  gemini: gemini-3-pro
  qoder: qoder-max
effort:
  claude: high
  copilot: xhigh
  qoder: max
`)
	mcpDir := filepath.Join(source, "mcps")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "docs.yaml"), "name: docs\ncommand: docs-mcp\nargs: [--stdio]\nenv:\n  TOKEN: ${DOCS_TOKEN}\n")
	mustWriteGlobalTest(t, filepath.Join(mcpDir, "web.yaml"), "name: web\ntype: http\nurl: https://example.com/mcp\n")
	if _, warnings, err := runGlobalAgentTest("--only", targets); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	snapshot := map[string]string{}
	for _, rel := range []string{".claude/settings.json", ".codex/config.toml", ".copilot/settings.json", ".copilot/mcp-config.json", ".cursor/mcp.json", ".gemini/settings.json", ".qoder/settings.json"} {
		snapshot[rel] = readGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel)))
	}
	for _, path := range []string{settings, mcpDir, filepath.Join(source, "state")} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	if _, warnings, err := runImportGlobalTest(); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	_, warnings, err := runGlobalAgentTest("--only", targets)
	if err != nil {
		t.Fatalf("sync after import: %v\n%s", err, warnings)
	}
	for rel, want := range snapshot {
		if got := readGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel))); got != want {
			t.Errorf("%s changed after import:\n%s\nwant:\n%s", rel, got, want)
		}
	}
}

func TestImportGlobal_NeverReplacesASpec(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-6-luna\"\n")
	existing := filepath.Join(source, "settings", "imported.yaml")
	mustWriteGlobalTest(t, existing, "effort:\n  claude: low\n")

	_, warnings, err := runImportGlobalTest("codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readGlobalTest(t, existing); got != "effort:\n  claude: low\n" {
		t.Errorf("an existing spec must stay: %q", got)
	}
	if !strings.Contains(warnings, "skipped "+existing) {
		t.Errorf("the skip must be named:\n%s", warnings)
	}
	out, _, err := runImportGlobalTest("codex", "--dry-run")
	if err != nil || strings.Contains(out, "dry-run: write") {
		t.Errorf("nothing new to write: %v\n%s", err, out)
	}
	if _, _, err := runImportGlobalTest("aider"); err == nil {
		t.Error("a target without user settings or MCP must fail")
	}
}

func TestImportGlobal_LeavesWhatTheHomeProvides(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "local", "mcps", "secret.yaml"), "name: secret\ncommand: s\nenv:\n  TOKEN: sk-live-123\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "me.yaml"), "model:\n  codex: private\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "documentation.yaml"), "name: docs\ncommand: docs-mcp\n")
	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if _, warnings, err := runImportGlobalTest("codex"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, rel := range []string{"mcps/secret.yaml", "mcps/docs.yaml", "settings/imported.yaml"} {
		if _, err := os.Stat(filepath.Join(source, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s must not be written: %v", rel, err)
		}
	}
	_ = home
}

func TestImportGlobal_HandWrittenShapesRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	files := map[string]string{
		".cursor/mcp.json":         `{"mcpServers": {"s": {"command": "x"}, "r": {"url": "https://x/mcp", "headers": {"A": "b"}}}}`,
		".copilot/mcp-config.json": `{"mcpServers": {"a": {"type": "local", "command": "x", "args": [], "tools": ["*"]}, "b": {"command": "y"}}}`,
		".gemini/settings.json":    `{"mcpServers": {"g": {"url": "https://x", "type": "http"}, "h": {"httpUrl": "https://y"}}}`,
	}
	for rel, body := range files {
		mustWriteGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel)), body+"\n")
	}
	if _, warnings, err := runImportGlobalTest("cursor", "copilot", "gemini"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	for _, name := range []string{"s", "r", "a", "b", "g", "h"} {
		if _, err := os.Stat(filepath.Join(source, "mcps", name+".yaml")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, target := range []string{"cursor", "copilot", "gemini"} {
		if _, warnings, err := runGlobalAgentTest("--only", target); err != nil {
			t.Fatalf("sync %s after import: %v\n%s", target, err, warnings)
		}
	}
	// Each file keeps its own servers exactly as written; the other
	// tools' imported servers join it, as home specs reach every tool.
	for rel, body := range files {
		var before, after map[string]map[string]any
		if err := json.Unmarshal([]byte(body), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel)))), &after); err != nil {
			t.Fatal(err)
		}
		for name, server := range before["mcpServers"] {
			if !reflect.DeepEqual(after["mcpServers"][name], server) {
				t.Errorf("%s: %s = %v, want %v", rel, name, after["mcpServers"][name], server)
			}
		}
	}
}

func TestImportGlobal_SkipsServerThatWouldNotRoundTrip(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.a]\ncommand = \"x\"\n\n[mcp_servers.a.tools.t]\napproval_mode = \"approve\"\n")
	_, warnings, err := runImportGlobalTest("codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "mcps", "a.yaml")); err == nil {
		if _, w, err := runGlobalAgentTest("--only", "codex", "--check"); err != nil {
			t.Errorf("an imported server must round-trip: %v\n%s", err, w)
		}
		return
	}
	if !strings.Contains(warnings, "skipped MCP server a") {
		t.Errorf("a skipped server must be named:\n%s", warnings)
	}
}

func TestImportGlobal_OneServerAcrossToolsImportsOnce(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".copilot", "mcp-config.json"), `{"mcpServers": {"d": {"command": "x", "tools": ["*"]}}}`+"\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"d": {"command": "x"}}}`+"\n")
	_, warnings, err := runImportGlobalTest("copilot", "cursor")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if strings.Contains(warnings, "differently") {
		t.Errorf("one server in two spellings is not a clash:\n%s", warnings)
	}
	if got := readGlobalTest(t, filepath.Join(source, "mcps", "d.yaml")); strings.Contains(got, "tools") {
		t.Errorf("Copilot's default tools must not reach the spec:\n%s", got)
	}
	if _, w, err := runGlobalAgentTest("--only", "copilot,cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	for rel, want := range map[string]string{".copilot/mcp-config.json": `{"mcpServers": {"d": {"command": "x", "tools": ["*"]}}}`, ".cursor/mcp.json": `{"mcpServers": {"d": {"command": "x"}}}`} {
		if got := readGlobalTest(t, filepath.Join(home, filepath.FromSlash(rel))); got != want+"\n" {
			t.Errorf("%s: both tools must adopt the server as written:\n%s", rel, got)
		}
	}
}

func TestImportGlobal_RelativeRootFails(t *testing.T) {
	globalAgentTestHome(t)
	t.Setenv("CODEX_HOME", "rel")
	if _, _, err := runImportGlobalTest("codex"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v", err)
	}
}
