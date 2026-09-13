package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A ::target fence naming an unknown target is a silent trap: the
// paragraph drops from every entry-point file with no other signal.
// validate must flag it.
func TestLintEntryPointFences_FlagsUnknownTarget(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::target gemni\nTypo'd target name.\n::end\n")

	issues := lintEntryPointFences(".", &config.Config{})
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

	if issues := lintEntryPointFences(".", &config.Config{}); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

// No AGNOSTIC_AI.md at all (sync has not run yet) is not an issue: sync
// seeds it on the first run.
func TestLintEntryPointFences_NoFileNoIssue(t *testing.T) {
	testutil.TempCwd(t)

	if issues := lintEntryPointFences(".", &config.Config{}); len(issues) != 0 {
		t.Errorf("expected no issues for a missing file, got %v", issues)
	}
}

// A target from an external adapter (agnostic-ai-adapter-<name> on PATH)
// is enabled in config but absent from the built-in registry. Its fence
// must validate clean.
func TestLintEntryPointFences_AcceptsConfiguredExternalTarget(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::target foo\nExternal adapter block.\n::end\n")
	cfg := &config.Config{Targets: []string{"claude", "foo"}}

	if issues := lintEntryPointFences(".", cfg); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

// Cursor reads no entry-point file, so a ::target cursor block reaches
// nothing even though the name is valid.
func TestLintEntryPointFences_FlagsKnownTargetWithoutEntryPoint(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::target cursor\nNever read.\n::end\n")

	issues := lintEntryPointFences(".", &config.Config{})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "cursor") {
		t.Fatalf("expected one issue naming cursor, got %v", issues)
	}
}

// A target on the legacy rules-file layout gets no pointer entry point,
// so its fence reaches nothing either.
func TestLintEntryPointFences_FlagsTargetOnLegacyRulesFile(t *testing.T) {
	testutil.TempCwd(t)
	writeAgnosticFile(t, "Shared.\n\n::target gemini\nNever read.\n::end\n")
	cfg := &config.Config{Outputs: map[string]config.Output{"gemini": {RulesFile: "GEMINI.md"}}}

	if issues := lintEntryPointFences(".", cfg); len(issues) != 1 {
		t.Errorf("expected one issue, got %v", issues)
	}
}
