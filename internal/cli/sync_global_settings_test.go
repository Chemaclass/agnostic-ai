package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const globalDefaultsSettings = `model:
  claude: opus
  codex: gpt-6-luna
effort:
  claude: high
  codex: high
`

const handWrittenCodexConfig = `# Personal Codex config
approval_policy = "on-request" # asked every time

[projects."/src/app"]
trust_level = "trusted"

# MCP servers
[mcp_servers.docs]
command = "docs-mcp"
args = ["--port", "[8080]"]
`

const handWrittenClaudeSettings = `{
  "theme": "dark",
  "permissions": {
    "allow": ["Bash(ls:*)"]
  }
}
`

func readGlobalTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSyncGlobal_SettingsWriteModelAndEffortKeys(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "settings", "defaults.yaml")
	mustWriteGlobalTest(t, spec, globalDefaultsSettings)
	codexPath := filepath.Join(home, ".codex", "config.toml")
	claudePath := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, codexPath, handWrittenCodexConfig)
	mustWriteGlobalTest(t, claudePath, handWrittenClaudeSettings)

	if _, warnings, err := runGlobalAgentTest("--only", "codex,claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	wantCodex := `# Personal Codex config
approval_policy = "on-request" # asked every time
model = "gpt-6-luna"
model_reasoning_effort = "high"

[projects."/src/app"]
trust_level = "trusted"

# MCP servers
[mcp_servers.docs]
command = "docs-mcp"
args = ["--port", "[8080]"]
`
	if got := readGlobalTest(t, codexPath); got != wantCodex {
		t.Errorf("config.toml:\n%s\nwant:\n%s", got, wantCodex)
	}
	var claudeDoc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, claudePath)), &claudeDoc); err != nil {
		t.Fatal(err)
	}
	if claudeDoc["model"] != "opus" || claudeDoc["effortLevel"] != "high" || claudeDoc["theme"] != "dark" || claudeDoc["permissions"] == nil {
		t.Errorf("settings.json = %v", claudeDoc)
	}

	claudeFirst := readGlobalTest(t, claudePath)
	out, _, err := runGlobalAgentTest("--only", "codex,claude", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if strings.Contains(out, "config.toml") || strings.Contains(out, "settings.json") {
		t.Errorf("second run must plan no settings write:\n%s", out)
	}
	if _, _, err := runGlobalAgentTest("--only", "codex,claude"); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := readGlobalTest(t, codexPath); got != wantCodex {
		t.Errorf("second sync changed config.toml:\n%s", got)
	}
	if got := readGlobalTest(t, claudePath); got != claudeFirst {
		t.Errorf("second sync changed settings.json:\n%s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "codex,claude", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}

	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "codex,claude"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, codexPath); got != handWrittenCodexConfig {
		t.Errorf("removal must restore config.toml:\n%s\nwant:\n%s", got, handWrittenCodexConfig)
	}
	if got := readGlobalTest(t, claudePath); got != handWrittenClaudeSettings {
		t.Errorf("removal must restore settings.json:\n%s\nwant:\n%s", got, handWrittenClaudeSettings)
	}
}

func TestSyncGlobal_SettingsCreateMissingCodexConfig(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), globalDefaultsSettings)

	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	want := "model = \"gpt-6-luna\"\nmodel_reasoning_effort = \"high\"\n"
	if got := readGlobalTest(t, filepath.Join(home, ".codex", "config.toml")); got != want {
		t.Errorf("config.toml = %q, want %q", got, want)
	}
}

func TestSyncGlobal_SettingsAdoptMatchingValue(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), "model:\n  codex: gpt-6-luna\n")
	codexPath := filepath.Join(home, ".codex", "config.toml")
	existing := "model = \"gpt-6-luna\" # picked in /model\n\n[profiles.fast]\nmodel = \"mini\"\n"
	mustWriteGlobalTest(t, codexPath, existing)

	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if !strings.Contains(warnings, "adopted "+codexPath+" model") {
		t.Errorf("adoption must be named:\n%s", warnings)
	}
	if got := readGlobalTest(t, codexPath); got != existing {
		t.Errorf("adoption must not rewrite the file:\n%s", got)
	}
}

func TestSyncGlobal_SettingsConflictStopsUnlessBackup(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "settings", "defaults.yaml")
	mustWriteGlobalTest(t, spec, "model:\n  codex: gpt-6-luna\n")
	codexPath := filepath.Join(home, ".codex", "config.toml")
	existing := "model = \"gpt-6-sol\" # picked in /model\nsandbox_mode = \"workspace-write\"\n"
	mustWriteGlobalTest(t, codexPath, existing)

	_, _, err := runGlobalAgentTest("--only", "codex")
	if err == nil {
		t.Fatal("a hand-set value must stop the run")
	}
	for _, want := range []string{codexPath, "model", `"gpt-6-sol"`, `"gpt-6-luna"`, `codex: "gpt-6-sol"`, spec, "--backup"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if got := readGlobalTest(t, codexPath); got != existing {
		t.Errorf("a stopped run must write nothing:\n%s", got)
	}

	if _, warnings, err := runGlobalAgentTest("--only", "codex", "--backup"); err != nil {
		t.Fatalf("backup sync: %v\n%s", err, warnings)
	}
	want := "model = \"gpt-6-luna\" # picked in /model\nsandbox_mode = \"workspace-write\"\n"
	if got := readGlobalTest(t, codexPath); got != want {
		t.Errorf("config.toml = %q, want %q", got, want)
	}
	if got := readGlobalTest(t, codexPath+".bak"); got != existing {
		t.Errorf(".bak = %q, want the original", got)
	}

	// The /model picker saving a new choice is drift, caught by the
	// next run rather than overwritten.
	mustWriteGlobalTest(t, codexPath, "model = \"gpt-6-mini\"\nsandbox_mode = \"workspace-write\"\n")
	if _, _, err := runGlobalAgentTest("--only", "codex", "--check"); err == nil {
		t.Error("check must fail on a changed managed key")
	}
	if _, _, err := runGlobalAgentTest("--only", "codex"); err == nil || !strings.Contains(err.Error(), `codex: "gpt-6-mini"`) {
		t.Errorf("sync must stop and print the adopting spec line: %v", err)
	}
}

func TestSyncGlobal_SettingsLocalLayerWins(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), globalDefaultsSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "machine.yaml"), "model:\n  codex: gpt-6-mini\n")

	if _, warnings, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	want := "model = \"gpt-6-mini\"\nmodel_reasoning_effort = \"high\"\n"
	if got := readGlobalTest(t, filepath.Join(home, ".codex", "config.toml")); got != want {
		t.Errorf("config.toml = %q, want %q", got, want)
	}
}

func TestSyncGlobal_SettingsKeepClaudeHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), globalDefaultsSettings)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -l .\n")

	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "opus" || doc["effortLevel"] != "high" || doc["hooks"] == nil {
		t.Errorf("settings.json must carry both hooks and settings: %v", doc)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}

func TestSyncGlobal_SettingsNotesWhatItCannotWrite(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "defaults.yaml"), `model: opus
effort:
  claude: max
permissions:
  allow: [Read]
x-claude:
  theme: dark
`)
	_, warnings, err := runGlobalAgentTest("--only", "claude,gemini")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	for _, want := range []string{"permissions", "x-claude", "max", "gemini"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "opus" || doc["effortLevel"] != nil || doc["permissions"] != nil || doc["theme"] != nil {
		t.Errorf("settings.json = %v", doc)
	}
}
