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
  hooks:
    Stop: []
`)
	_, warnings, err := runGlobalAgentTest("--only", "claude,gemini")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	for _, want := range []string{"permissions", "x-claude.hooks", "max", "gemini"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("warnings lack %q:\n%s", want, warnings)
		}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "opus" || doc["effortLevel"] != nil || doc["permissions"] != nil || doc["hooks"] != nil || doc["theme"] != "dark" {
		t.Errorf("settings.json = %v", doc)
	}
}

func TestSyncGlobal_SettingsReplaceLastClaudeHook(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo hi\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model: opus\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, filepath.Join(home, ".claude", "settings.json"))
	if strings.Contains(got, "hooks") || strings.Contains(got, "AGNOSTIC_AI_TARGET") || !strings.Contains(got, `"model": "opus"`) {
		t.Errorf("the removed hook must go and the model stay:\n%s", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}

func TestSyncGlobal_DryRunNamesHookWriteBesideAdoptedSetting(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model:\n  claude: opus\n")
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, settings, "{\n  \"model\": \"opus\"\n}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "start.yaml"), "event: SessionStart\ncommand: echo hi\n")

	out, _, err := runGlobalAgentTest("--only", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(out, "dry-run: write "+settings) {
		t.Errorf("dry-run must name the hooks write:\n%s", out)
	}
}

func TestSyncGlobal_WritesThroughSymlinkedUserFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be brief.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model: opus\n")
	dotfiles := filepath.Join(home, "dotfiles")
	realClaude := filepath.Join(dotfiles, "CLAUDE.md")
	realSettings := filepath.Join(dotfiles, "settings.json")
	mustWriteGlobalTest(t, realClaude, "Mine.\n")
	mustWriteGlobalTest(t, realSettings, "{\n  \"theme\": \"dark\"\n}\n")
	link := filepath.Join(home, ".claude", "CLAUDE.md")
	settingsLink := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realClaude, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(realSettings, settingsLink); err != nil {
		t.Fatal(err)
	}

	_, warnings, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	for _, path := range []string{link, settingsLink} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s must stay a symlink: %v", path, err)
		}
	}
	if got := readGlobalTest(t, realClaude); !strings.Contains(got, "Mine.") || !strings.Contains(got, "Be brief.") {
		t.Errorf("the link target must get the managed block:\n%s", got)
	}
	if got := readGlobalTest(t, realSettings); !strings.Contains(got, `"model": "opus"`) || !strings.Contains(got, `"theme": "dark"`) {
		t.Errorf("the link target must get the model:\n%s", got)
	}
	if !strings.Contains(warnings, "wrote through symlink "+link) {
		t.Errorf("the write through a link must be named:\n%s", warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}

func TestSyncGlobal_SettingsPassThroughTargetKeys(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "settings", "d.yaml")
	mustWriteGlobalTest(t, spec, `model: opus
x-claude:
  model: sonnet
  alwaysThinkingEnabled: true
  statusLine:
    type: command
    command: ~/bin/status
x-codex:
  model_reasoning_summary: concise
  notify: [notify-send, done]
  profiles:
    fast:
      model: mini
`)
	claudePath := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, claudePath, "{\n  \"statusLine\": {\n    \"padding\": 1\n  }\n}\n")
	codexPath := filepath.Join(home, ".codex", "config.toml")
	mustWriteGlobalTest(t, codexPath, "[tui]\ntheme = \"dark\"\n")

	_, warnings, err := runGlobalAgentTest("--only", "claude,codex")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, claudePath)), &doc); err != nil {
		t.Fatal(err)
	}
	status, _ := doc["statusLine"].(map[string]any)
	if doc["model"] != "sonnet" || doc["alwaysThinkingEnabled"] != true || status["command"] != "~/bin/status" || status["padding"] != float64(1) {
		t.Errorf("settings.json = %v", doc)
	}
	wantCodex := "model = \"opus\"\nmodel_reasoning_summary = \"concise\"\nnotify = [\"notify-send\", \"done\"]\n\n[tui]\ntheme = \"dark\"\n"
	if got := readGlobalTest(t, codexPath); got != wantCodex {
		t.Errorf("config.toml = %q, want %q", got, wantCodex)
	}
	if !strings.Contains(warnings, "x-codex.profiles") {
		t.Errorf("a table must raise a note:\n%s", warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude,codex", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}

	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, claudePath); got != "{\n  \"statusLine\": {\n    \"padding\": 1\n  }\n}\n" {
		t.Errorf("removal must restore settings.json:\n%s", got)
	}
	if got := readGlobalTest(t, codexPath); got != "[tui]\ntheme = \"dark\"\n" {
		t.Errorf("removal must restore config.toml:\n%s", got)
	}
}

func TestSyncGlobal_SettingsReachGeminiQoderCopilot(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), `model:
  gemini: gemini-3-pro
  qoder: qoder-max
  copilot: gpt-6-luna
effort:
  qoder: max
  copilot: xhigh
  gemini: high
`)
	geminiPath := filepath.Join(home, ".gemini", "settings.json")
	mustWriteGlobalTest(t, geminiPath, "{\n  \"model\": {\n    \"maxSessionTurns\": 20\n  }\n}\n")

	_, warnings, err := runGlobalAgentTest("--only", "gemini,qoder,copilot")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	read := func(path string) map[string]any {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	gemini := read(geminiPath)["model"].(map[string]any)
	if gemini["name"] != "gemini-3-pro" || gemini["maxSessionTurns"] != float64(20) {
		t.Errorf("gemini model = %v", gemini)
	}
	qoder := read(filepath.Join(home, ".qoder", "settings.json"))["model"].(map[string]any)
	if qoder["name"] != "qoder-max" || qoder["reasoningEffort"] != "max" {
		t.Errorf("qoder model = %v", qoder)
	}
	copilot := read(filepath.Join(home, ".copilot", "settings.json"))
	if copilot["model"] != "gpt-6-luna" || copilot["effortLevel"] != "xhigh" {
		t.Errorf("copilot settings = %v", copilot)
	}
	if !strings.Contains(warnings, "gemini") || !strings.Contains(warnings, "effort") {
		t.Errorf("gemini effort must raise a note:\n%s", warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "gemini,qoder,copilot", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}
