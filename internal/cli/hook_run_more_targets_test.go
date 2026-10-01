package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// One shared script synced to Trae, OpenHands, Goose, and Augment blocks
// a force push on each, reading each target's payload and env.
func TestHookRun_SharedScriptBlocksOnTraeOpenHandsGooseAndAugment(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [trae, openhands, goose, augment]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, script, `#!/bin/sh
payload=$(cat)
echo "env:${AGNOSTIC_AI_TARGET:-}${TRAE_PROJECT_DIR:+trae}${OPENHANDS_EVENT_TYPE:+openhands}${PLUGIN_ROOT:+goose}${AUGMENT_HOOK_EVENT:+augment}" >&2
case "$payload" in
  *'git push --force'*) echo "no force push" >&2; exit 2 ;;
esac
exit 0
`)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		"trae: block (exit 2", "event: PreToolUse (RunCommand)", "env:trae",
		"openhands: block (exit 2", "event: PreToolUse (terminal)", "env:openhands",
		"goose: block (exit 2", "event: PreToolUse (shell)", "env:goosegoose",
		"augment: block (exit 2", "event: PreToolUse (launch-process)", "env:augment",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("a fresh sync warns:\n%s", out)
	}

	out, err = runHookRun(t, "guard", "--bash", "git status", "--expect", "allow")
	if err != nil {
		t.Fatalf("git status: err = %v\n%s", err, out)
	}
}

func TestHookRun_AugmentSkipsAnInlineCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [augment, trae]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: 'exit 0'\n")

	out, err := runHookRun(t, "guard", "--bash", "ls")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "augment: not run (Augment runs only a hook command that is a .sh") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_GooseFailClosedTurnsAFailureIntoABlock(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: PreToolUse\nx-goose:\n  on_failure: block\ncommand: 'echo not-json; exit 1'\n")

	out, err := runHookRun(t, "guard", "--bash", "ls", "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
}
