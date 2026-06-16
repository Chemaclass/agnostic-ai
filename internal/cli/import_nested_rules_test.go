package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// agnostic now emits scoped Claude rules under .claude/rules/<scope>/.
// Importing them back must preserve the nesting instead of dropping the
// subdirectory files (the #411 bug, for the claude target).
func TestImportFromClaude_WalksNestedRuleSubdirectories(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	mustWrite(t, filepath.Join(dir, ".claude/rules/backend/api/auth.md"), "---\nname: auth\n---\n\nbackend rule\n")
	mustWrite(t, filepath.Join(dir, ".claude/rules/top.md"), "---\nname: top\n---\n\ntop rule\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules/backend/api/auth.md")); err != nil {
		t.Errorf("nested rule not imported under rules/backend/api: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules/top.md")); err != nil {
		t.Errorf("top-level rule missing: %v", err)
	}
}

// Copilot emits scoped instructions under .github/instructions/<scope>/.
// The importer must walk the tree and keep the scope, not flatten or drop
// nested files.
func TestImportFromCopilot_WalksNestedInstructionSubdirectories(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	mustWrite(t, filepath.Join(dir, ".github/instructions/backend/auth.instructions.md"), "---\nname: auth\n---\n\nbackend rule\n")
	mustWrite(t, filepath.Join(dir, ".github/instructions/top.instructions.md"), "---\nname: top\n---\n\ntop rule\n")

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules/backend/auth.md")); err != nil {
		t.Errorf("nested instruction not imported under rules/backend: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules/top.md")); err != nil {
		t.Errorf("top-level instruction missing: %v", err)
	}
}
