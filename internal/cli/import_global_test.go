package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
	mustWriteGlobalTest(t, existing, "model: mine\n")

	_, warnings, err := runImportGlobalTest("codex")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readGlobalTest(t, existing); got != "model: mine\n" {
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
