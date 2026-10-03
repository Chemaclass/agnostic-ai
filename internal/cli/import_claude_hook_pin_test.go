package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const claudeHookPinSettings = `{"hooks":{
  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}],
  "PostToolUse":[{"matcher":"Edit|MultiEdit","hooks":[{"type":"command","command":"fmt.sh"}]}],
  "Notification":[{"hooks":[{"type":"command","command":"notify.sh"}]}]
}}`

func importedHookTargets(t *testing.T, dir string) map[string]any {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, e := range entries {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(readFileString(t, filepath.Join(dir, e.Name()))), &doc); err != nil {
			t.Fatal(err)
		}
		out[doc["event"].(string)] = doc["target"]
	}
	return out
}

// In a claude,codex project a hook Codex runs as written stays portable;
// one it cannot run keeps the pin, and the summary says why (#1328).
func TestImportClaudeHooks_UnpinsWhatCodexRuns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), claudeHookPinSettings)
	log := captureLog(t)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := importClaudeHooks(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := importedHookTargets(t, dst)
	if got["PreToolUse"] != nil {
		t.Errorf("Bash hook pinned to %v, want it portable", got["PreToolUse"])
	}
	for _, event := range []string{"PostToolUse", "Notification"} {
		if got[event] != "claude" {
			t.Errorf("%s hook target = %v, want claude", event, got[event])
		}
	}
	for _, want := range []string{`Codex PostToolUse does not match "MultiEdit"`, "Codex has no Notification event"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, log.String())
		}
	}
}

// Without another target that can judge a hook, every hook keeps the pin
// and the summary stays quiet, as before.
func TestImportClaudeHooks_PinsWhenNoTargetCanJudge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), claudeHookPinSettings)
	log := captureLog(t)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := importClaudeHooks(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	for event, target := range importedHookTargets(t, dst) {
		if target != "claude" {
			t.Errorf("%s hook target = %v, want claude", event, target)
		}
	}
	if strings.Contains(log.String(), "keeps target") {
		t.Errorf("summary explains a pin no target judged:\n%s", log.String())
	}
}

// A third target that cannot tell keeps the pin even when Codex runs the
// hook, since the hook would reach that target too.
func TestImportClaudeHooks_PinsWhenAnotherTargetCannotTell(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}]}}`)
	log := captureLog(t)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := importClaudeHooks(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := importedHookTargets(t, dst)["PreToolUse"]; got != "claude" {
		t.Errorf("target = %v, want claude", got)
	}
	if !strings.Contains(log.String(), "cannot tell whether cursor runs it") {
		t.Errorf("summary missing the reason:\n%s", log.String())
	}
}

// An anchored, grouped matcher names Bash and exec, so Codex runs it and
// the hook stays portable (#1733).
func TestImportClaudeHooks_UnpinsGroupedMatcher(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"^(Bash|exec)$","hooks":[{"type":"command","command":"exit 0"}]}]}}`)
	log := captureLog(t)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := importClaudeHooks(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := importedHookTargets(t, dst)["PreToolUse"]; got != nil {
		t.Errorf("grouped Bash hook pinned to %v, want it portable\n%s", got, log.String())
	}
}
