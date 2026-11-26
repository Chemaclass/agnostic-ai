package integration

import (
	"strings"
	"testing"
)

func TestDoctor_FlagsIgnoredOutputsUntilHookAndCommitFixThem(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, copilot, jules, gemini]\ngitignore:\n  enabled: true\n")
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("sync")

	out := run("doctor")
	for _, want := range []string{
		"Clones and cloud agents:",
		"nothing runs sync after a pull or branch switch",
		"→ agnostic-ai install-hook --post-checkout",
		"AGENTS.md is ignored, so Jules starts without project instructions",
		"CLAUDE.md is ignored, so Claude Code on the web starts",
		".github/copilot-instructions.md is ignored, so Copilot cloud agent starts",
		"gitignore.commit: [instructions]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "All checks passed") || strings.Contains(out, "GEMINI.md is ignored") {
		t.Errorf("doctor must not claim all passed or name a tool without a cloud agent:\n%s", out)
	}

	run("install-hook", "--post-checkout")
	out = run("doctor")
	if strings.Contains(out, "nothing runs sync") || !strings.Contains(out, "CLAUDE.md is ignored") {
		t.Errorf("the hook settles freshness but not the cloud agents:\n%s", out)
	}

	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, copilot, jules, gemini]\ngitignore:\n  enabled: true\n  commit: [instructions]\n")
	run("sync")
	out = run("doctor")
	if strings.Contains(out, "Clones and cloud agents") || !strings.Contains(out, "All checks passed") {
		t.Errorf("doctor must stay quiet once the root files leave the ignore block:\n%s", out)
	}
}

func TestLint_WarnsOnMissingProjectMemoryIndexUntilMemoryIndexCreatesIt(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")

	out, err := ellipsisCommand(t, binary, dir, "lint")
	if err != nil || !strings.Contains(out, "LINT067 [warn]") {
		t.Fatalf("lint must warn without failing (err %v):\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "memory", "index"); err != nil || !strings.Contains(out, "created") {
		t.Fatalf("memory index (err %v):\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "lint"); err != nil || strings.Contains(out, "LINT067") {
		t.Errorf("the warning must clear (err %v):\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "explain", "LINT067"); err != nil || !strings.Contains(out, "agnostic-ai memory index") {
		t.Errorf("explain must name the fix (err %v):\n%s", err, out)
	}
}

func TestInit_ClosingNotesCoverFreshnessAndCloudAgents(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	handoffGit(t, dir, "init", "-q")

	out, err := ellipsisCommand(t, binary, dir, "init", "--all")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	for _, want := range []string{"each clone needs agnostic-ai sync", "agnostic-ai install-hook --post-checkout", "cloud agents read only committed files", "sync-settings/#gitignore"} {
		if !strings.Contains(out, want) {
			t.Errorf("init output lacks %q:\n%s", want, out)
		}
	}
}
