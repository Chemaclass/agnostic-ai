package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_WarnsWhenAProtectedPathCoversAFileSyncWrites(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/settings/protected.yaml"),
		"protected:\n  paths: [.claude/**, composer.lock]\n  reason: Hands off.\n")
	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("the finding is a warning: %v\n%s", err, out)
	}
	lines := strings.Join(findingLines(out, "LINT022"), "\n")
	for _, want := range []string{"protected.yaml", ".claude/**", ".claude/settings.json"} {
		if !strings.Contains(lines, want) {
			t.Errorf("LINT022 lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(lines, "composer.lock") {
		t.Errorf("a path sync does not write was flagged:\n%s", out)
	}
	if _, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Error("lint --strict should fail on LINT022")
	}
}

func TestLint_ProtectedPathsSyncDoesNotWriteAreClean(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/settings/protected.yaml"),
		"protected:\n  paths: [.github/**, composer.lock]\n")
	out, err := runCLI(t, "lint", "--strict")
	if err != nil || len(findingLines(out, "LINT022")) != 0 {
		t.Errorf("unexpected findings: %v\n%s", err, out)
	}
}

func TestLint_ReportsAnInvalidProtectedBlock(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/settings/protected.yaml"),
		"protected:\n  paths: [\"../outside\"]\n")
	out, err := runCLI(t, "lint")
	if err == nil || len(findingLines(out, "LINT023")) != 1 {
		t.Errorf("want one LINT023 error: %v\n%s", err, out)
	}
}
