package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A hook that prints {"decision": "deny"} and exits 0 blocks on every
// tool that maps before-tool and can run the wrapper; Augment runs only a
// bare script path, so validate names it.
func TestHookRun_StdoutDecisionBlocksOnEveryBeforeToolTarget(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	targets := []string{"claude", "codex", "gemini", "factory", "qoder", "openhands", "goose", "crush", "windsurf", "cursor", "copilot", "cline"}
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(targets, ", ")+"]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, script, "#!/bin/sh\nif grep -q \"push --force\"; then echo '{\"decision\": \"deny\", \"reason\": \"no force push\"}'; else echo '{\"decision\": \"allow\"}'; fi\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ndecision: stdout\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block", "--include-assumed")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, target := range targets {
		if !strings.Contains(out, target+": block (") {
			t.Errorf("%s must block:\n%s", target, out)
		}
	}
	if !strings.Contains(out, "stderr: no force push") {
		t.Errorf("the reason must reach stderr:\n%s", out)
	}
	if allowed, err := runHookRun(t, "guard", "--bash", "git push", "--expect", "allow", "--include-assumed"); err != nil {
		t.Errorf("an allow decision must pass everywhere: %v\n%s", err, allowed)
	}

	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, augment]\n")
	if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, "augment runs a hook command only as a bare script path") {
		t.Errorf("validate must name augment: %v\n%s", err, out)
	}
}

// Cursor runs .claude/settings.json hooks too, so the Claude copy of a
// portable hook Cursor also gets exits 0 under Cursor and runs once there,
// as Cursor's own copy. Claude Code, and a native hook, run as before.
func TestSync_PortableHookRunsOnceOnCursorBesideClaude(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
	hooks := filepath.Join(dir, ".agnostic-ai", "hooks")
	mustWrite(t, filepath.Join(hooks, "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ncommand: 'echo blocked >&2; exit 2'\n")
	mustWrite(t, filepath.Join(hooks, "audit.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: 'echo audit'\n")
	mustWrite(t, filepath.Join(hooks, "status.yaml"), "on: stop\ntarget: claude\ncommand: 'echo stop'\n")
	mustSync(t)

	settings := readGlobalJSON(t, filepath.Join(dir, ".claude", "settings.json"))
	var guarded, audit string
	for _, group := range settings["hooks"].(map[string]any)["PreToolUse"].([]any) {
		for _, h := range group.(map[string]any)["hooks"].([]any) {
			command := h.(map[string]any)["command"].(string)
			if strings.Contains(command, "audit") {
				audit = command
			} else {
				guarded = command
			}
		}
	}
	if stop := firstGlobalHandler(t, settings, "Stop")["command"]; audit != "echo audit" || stop != "echo stop" {
		t.Errorf("a native hook, and one that does not reach Cursor, stay as written: %q %q", audit, stop)
	}
	for env, want := range map[string]int{"cursor": 0, "claude": 2, "": 2} {
		cmd := exec.Command("bash", "-c", guarded)
		cmd.Env = append(os.Environ(), "AGNOSTIC_AI_TARGET="+env)
		err := cmd.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		if code != want {
			t.Errorf("AGNOSTIC_AI_TARGET=%q: %q exits %d, want %d", env, guarded, code, want)
		}
	}
	out, err := runHookRun(t, "guard", "--bash", "ls", "--expect", "block", "--include-assumed")
	if err != nil || strings.Contains(out, "warning:") {
		t.Errorf("hook run must block on both, with no drift: %v\n%s", err, out)
	}

	imported := t.TempDir()
	if _, err := importClaudeHooks(dir, imported); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(imported)
	for _, e := range entries {
		body, _ := os.ReadFile(filepath.Join(imported, e.Name()))
		if strings.Contains(string(body), "AGNOSTIC_AI_TARGET") {
			t.Errorf("import must drop the Cursor guard:\n%s", body)
		}
	}
}
