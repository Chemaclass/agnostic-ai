package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readHookSpec(t *testing.T, dir, prefix string) string {
	t.Helper()
	data, err := os.ReadFile(findOneHookFile(t, filepath.Join(dir, "hooks"), prefix))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestImportFromCodex_StripsTheTargetSyncAdded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "hooks.json"), `{"hooks": {"Stop": [{"matcher": "", "hooks": [
  {"type": "command", "command": "export AGNOSTIC_AI_TARGET=codex; done.sh", "commandWindows": "done.sh"}
]}]}}`)
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readHookSpec(t, dir, "stop")
	if !strings.Contains(got, "command: done.sh") || strings.Contains(got, "AGNOSTIC_AI_TARGET") || strings.Contains(got, "commandWindows") {
		t.Errorf("imported spec:\n%s", got)
	}
}

func TestImportFromGemini_DropsOnlyTheTargetEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, geminiSettings), `{"hooks": {"BeforeTool": [{"hooks": [
  {"type": "command", "command": "guard.sh", "env": {"AGNOSTIC_AI_TARGET": "gemini", "FOO": "bar"}}
]}]}}`)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readHookSpec(t, dir, "beforetool")
	if strings.Contains(got, "AGNOSTIC_AI_TARGET") || !strings.Contains(got, "FOO: bar") {
		t.Errorf("imported spec:\n%s", got)
	}
}

func TestImportClaudeSettingsOverlay_DropsTheTargetEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, claudeDir, "settings.json"), `{"model": "opus", "env": {"AGNOSTIC_AI_TARGET": "claude"}, "hooks": {}}`)
	if _, _, err := importClaudeSettingsOverlay(dir, filepath.Join(dir, "settings")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(claudeOverlayPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "env") || !strings.Contains(string(data), "opus") {
		t.Errorf("overlay:\n%s", data)
	}
}

func TestImportClaudeSettingsOverlay_KeepsTheUsersEnvOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, claudeDir, "settings.json"), `{"env": {"ZED": "1", "AGNOSTIC_AI_TARGET": "claude", "ALPHA": "2"}}`)
	if _, _, err := importClaudeSettingsOverlay(dir, filepath.Join(dir, "settings")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(claudeOverlayPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "AGNOSTIC_AI_TARGET") || strings.Index(got, `"ZED"`) > strings.Index(got, `"ALPHA"`) {
		t.Errorf("overlay:\n%s", got)
	}
}

// Folded args come back as one shell-form command, which syncs to the
// same hooks.json.
func TestImportFromCodex_FoldedArgsStayOneCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex", "hooks.json"), `{"hooks": {"Stop": [{"matcher": "", "hooks": [
  {"type": "command", "command": "export AGNOSTIC_AI_TARGET=codex; bash 'guard.sh'", "commandWindows": "bash 'guard.sh'"}
]}]}}`)
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readHookSpec(t, dir, "stop")
	if !strings.Contains(got, "bash 'guard.sh'") || strings.Contains(got, "args") || strings.Contains(got, "commandWindows") {
		t.Errorf("imported spec:\n%s", got)
	}
}

// Folded args come back from Gemini as one shell-form command.
func TestImportFromGemini_FoldedArgsStayOneCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, geminiSettings), `{"hooks": {"BeforeTool": [{"hooks": [
  {"type": "command", "command": "node 'guard.js'", "env": {"AGNOSTIC_AI_TARGET": "gemini"}}
]}]}}`)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readHookSpec(t, dir, "beforetool")
	if !strings.Contains(got, "node 'guard.js'") || strings.Contains(got, "args") {
		t.Errorf("imported spec:\n%s", got)
	}
}
