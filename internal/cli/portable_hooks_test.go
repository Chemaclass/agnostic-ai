package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func TestValidateAndLint_ReportInvalidPortableHooks(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	hooks := filepath.Join(dir, ".agnostic-ai", "hooks")
	mustWrite(t, filepath.Join(hooks, "typo.yaml"), "on: befor-tool\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "read.yaml"), "on: before-tool\nmatch: read\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "both.yaml"), "on: stop\nevent: Stop\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "ok.yaml"), "on: before-tool\nmatch: read\ntarget-exclude: codex\ncommand: x\n")

	out, err := runCLI(t, "validate")
	if err == nil {
		t.Fatalf("validate must fail:\n%s", out)
	}
	lint, _ := runCLI(t, "lint")
	for _, want := range []string{
		`typo.yaml: unknown hook event "befor-tool" for on: (did you mean before-tool?)`,
		"read.yaml: codex has no read tool a hook can match; add target-exclude: codex or use event:",
		"both.yaml: sets both on: and event:; keep one",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("validate misses %q:\n%s", want, out)
		}
		if !strings.Contains(lint, want) {
			t.Errorf("lint misses %q:\n%s", want, lint)
		}
	}
	if strings.Contains(out, "ok.yaml") || strings.Contains(lint, "ok.yaml") || len(findingLines(lint, "LINT032")) != 3 {
		t.Errorf("want three LINT032 findings, none for ok.yaml:\n%s\n%s", out, lint)
	}
}

func TestSync_StopsOnAnInvalidPortableHook(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "on: before-tool\nmatch: shel\ncommand: x\n")
	if _, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "did you mean shell") {
		t.Errorf("sync = %v, want it to stop on the typo", err)
	}
}

func TestSync_PortableHookReachesClaudeAndCodexAndNotesTheRest(t *testing.T) {
	dir := newProject(t)
	captureLogOut(t)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "on: before-tool\nmatch: shell\ncommand: exit 2\n")
	mustSync(t)

	for _, path := range []string{".claude/settings.json", ".codex/hooks.json"} {
		body, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !strings.Contains(string(body), `"PreToolUse"`) || !strings.Contains(string(body), `"matcher": "Bash"`) {
			t.Errorf("%s = %s, %v; want a PreToolUse Bash hook", path, body, err)
		}
	}
	if body, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json")); err == nil && strings.Contains(string(body), "exit 2") {
		t.Errorf("cursor got the portable hook before it translates: %s", body)
	}
	if !strings.Contains(notes.String(), "1 hook reaches cursor only in the source dir (on: is translated for claude and codex only so far; write event: for cursor)") {
		t.Errorf("notes = %q, want one cursor note", notes.String())
	}
}

func TestHookRun_PortableHookRunsOnClaudeAndCodex(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\non: before-tool\nmatch: shell\n"+
		`command: 'if grep -q "push --force"; then echo "no force push" >&2; exit 2; fi'`+"\n")

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		"claude: block (exit 2", "codex: block (exit 2", "event: PreToolUse (Bash)",
		"cursor: not run (on: is translated for claude and codex only so far; write event: for cursor)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestHookRun_PortableEditHookRunsOnEveryEditTool(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\non: before-tool\nmatch: edit\ncommand: 'echo protected >&2; exit 2'\n")

	out, err := runHookRun(t, "guard", "--edit", "src/app.go", "--expect", "block", "--target", "claude,codex")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{"claude: block (exit 2", "codex: block (exit 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestSyncGlobal_StopsOnAnInvalidPortableHook(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\non: befor-tool\ncommand: 'true'\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "did you mean before-tool") {
		t.Errorf("sync --global = %v, want it to stop on the typo", err)
	}
}

func TestSyncGlobal_PortableHookReachesClaudeAndNotesCursor(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ncommand: 'exit 2'\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,cursor")
	if err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	if got := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".claude", "settings.json")), "PreToolUse")["command"]; got != "exit 2" {
		t.Errorf("claude PreToolUse handler = %v", got)
	}
	if !strings.Contains(warnings, "1 hook reaches cursor only in the source dir (on: is translated for claude and codex only so far; write event: for cursor)") {
		t.Errorf("want one cursor note:\n%s", warnings)
	}
}
