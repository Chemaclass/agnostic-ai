package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const unmanagedBaseConfig = "version: 1\ntargets: [claude, cursor]\ngitignore:\n  enabled: true\n"

// setupUnmanagedProject reuses setupFixture's specs with a two-target
// config and moves into the project.
func setupUnmanagedProject(t *testing.T) string {
	t.Helper()
	dir := setupFixture(t)
	writeUnmanagedConfig(t, dir)
	testutil.Chdir(t, dir)
	return dir
}

// writeUnmanagedConfig rewrites agnostic-ai.yaml, listing paths under
// sync.unmanaged. No paths writes the base config alone.
func writeUnmanagedConfig(t *testing.T, dir string, paths ...string) {
	t.Helper()
	body := unmanagedBaseConfig
	if len(paths) > 0 {
		body += "sync:\n  unmanaged:\n    - " + strings.Join(paths, "\n    - ") + "\n"
	}
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(body), 0o644))
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

const ownedRule = ".cursor/rules/sample-rule.mdc"

// handOwnRule syncs once, lists the cursor rule as user-owned, hand-edits
// it, and syncs again: the state the removal scenario starts from.
func handOwnRule(t *testing.T, dir string) {
	t.Helper()
	runCmd(t, "sync")
	writeUnmanagedConfig(t, dir, ownedRule)
	must(t, os.WriteFile(ownedRule, []byte("hand-owned\n"), 0o644))
	runCmd(t, "sync")
}

func TestUnmanaged_HandEditSurvivesSync(t *testing.T) {
	dir := setupUnmanagedProject(t)
	handOwnRule(t, dir)

	if got := readString(t, ownedRule); got != "hand-owned\n" {
		t.Errorf("sync rewrote the user-owned rule: %q", got)
	}
	runCmd(t, "sync", "--check")
	runCmd(t, "doctor")
	if state := readString(t, ".agnostic-ai/.sync-state"); strings.Contains(state, ownedRule) {
		t.Errorf("user-owned path in the sync ledger:\n%s", state)
	}

	runCmd(t, "sync")
	if got := readString(t, ownedRule); got != "hand-owned\n" {
		t.Errorf("second sync rewrote the user-owned rule: %q", got)
	}
}

func TestUnmanaged_GitignoreKeepsPreciseEntries(t *testing.T) {
	dir := setupUnmanagedProject(t)
	writeUnmanagedConfig(t, dir, ".claude/agents/sample-agent.md")

	runCmd(t, "sync")

	gitignore := readString(t, ".gitignore")
	lines := strings.Split(gitignore, "\n")
	for _, l := range lines {
		if l == "/.claude/agents/" || strings.Contains(l, "sample-agent.md") {
			t.Errorf(".gitignore hides the user-owned agent with %q:\n%s", l, gitignore)
		}
	}
	if !strings.Contains(gitignore, "\n/.claude/rules/\n") {
		t.Errorf(".gitignore should still collapse /.claude/rules/:\n%s", gitignore)
	}
}

func TestUnmanaged_RemovingEntryRestoresManagement(t *testing.T) {
	dir := setupUnmanagedProject(t)
	handOwnRule(t, dir)
	writeUnmanagedConfig(t, dir)

	runCmd(t, "sync")

	if got := readString(t, ownedRule); !strings.Contains(got, header.Marker) {
		t.Errorf("sync should manage the rule again, got:\n%s", got)
	}
	runCmd(t, "sync", "--check")
	for _, p := range []string{".claude/agents/sample-agent.md", "CLAUDE.md"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should still exist: %v", p, err)
		}
	}
}
