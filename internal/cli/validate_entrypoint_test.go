package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A ::target fence naming an unknown target is a silent trap: the
// paragraph drops from every entry-point file with no other signal.
// validate must flag it.
func TestLintEntryPointFences_FlagsUnknownTarget(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::target gemni\nTypo'd target name.\n::end\n")

	issues := lintEntryPointFences(".")
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 issue, got %d: %v", len(issues), issues)
	}
	if !strings.Contains(issues[0].Message, "gemni") {
		t.Errorf("issue message must name the unknown target %q: %s", "gemni", issues[0].Message)
	}
}

// Fences naming only known targets validate clean.
func TestLintEntryPointFences_CleanForKnownTargets(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::targets claude codex\nFine.\n::end\n")

	if issues := lintEntryPointFences("."); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

// No AGNOSTIC_AI.md at all (sync has not run yet) is not an issue: sync
// seeds it on the first run.
func TestLintEntryPointFences_NoFileNoIssue(t *testing.T) {
	testutil.TempCwd(t)

	if issues := lintEntryPointFences("."); len(issues) != 0 {
		t.Errorf("expected no issues for a missing file, got %v", issues)
	}
}
