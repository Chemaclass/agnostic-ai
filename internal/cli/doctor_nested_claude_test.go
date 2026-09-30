package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const nestedClaudeRuleA = "---\nname: a\nglobs: src/a/**\nscope: src/a\n---\n\n# Module A\n\nA facts.\n"

func TestDoctor_ListsNestedClaudeMDARuleHolds(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), nestedClaudeRuleA)
	nested := filepath.Join(dir, "src", "a", "CLAUDE.md")
	mustWrite(t, nested, "# Module A\n\nA facts.\n")

	out, err := runDoctor(t)
	if err == nil {
		t.Fatalf("doctor should fail while Claude Code loads the text twice, got:\n%s", out)
	}
	if !strings.Contains(out, "src/a/CLAUDE.md") || !strings.Contains(out, "doctor --fix") {
		t.Errorf("doctor should list src/a/CLAUDE.md with the fix, got:\n%s", out)
	}
	if !fileExists(nested) {
		t.Error("doctor without --fix must not remove the file")
	}
}

// In a Claude plus Codex project the hand-written CLAUDE.md beside the
// scoped AGENTS.md stops sync, so doctor --fix removes it first.
func TestDoctorFix_RemovesImportedNestedClaudeMDSoSyncPasses(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Root\n\nroot text\n")
	nested := filepath.Join(dir, "src", "a", "CLAUDE.md")
	mustWrite(t, nested, "# Module A\n\nA facts.\n\n## Tests\n\nRun make test.\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if out, err := runDoctor(t, "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}

	if fileExists(nested) {
		t.Fatal("doctor --fix should remove the nested CLAUDE.md a rule holds")
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync after doctor --fix: %v\n%s", err, out)
	}
	if got := mustRead(t, filepath.Join(dir, "src", "a", "AGENTS.md")); !strings.Contains(got, "Run make test.") {
		t.Errorf("src/a/AGENTS.md should hold the rule text, got %q", got)
	}
}

func TestDoctorFix_KeepsNestedClaudeMDThatDiffersFromEveryRule(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), nestedClaudeRuleA)
	nested := filepath.Join(dir, "src", "a", "CLAUDE.md")
	edited := "# Module A\n\nA facts.\n\nA line added after import.\n"
	mustWrite(t, nested, edited)
	elsewhere := filepath.Join(dir, "src", "c", "CLAUDE.md")
	mustWrite(t, elsewhere, "# Module A\n\nA facts.\n")

	out, _ := runDoctor(t, "--fix")

	if got, err := os.ReadFile(nested); err != nil || string(got) != edited {
		t.Errorf("doctor --fix must keep a nested CLAUDE.md whose text differs, got %q, err %v", got, err)
	}
	if !fileExists(elsewhere) {
		t.Error("doctor --fix must keep a CLAUDE.md in a directory no rule scopes")
	}
	if strings.Contains(out, "src/a/CLAUDE.md") || strings.Contains(out, "src/c/CLAUDE.md") {
		t.Errorf("doctor should not list a nested CLAUDE.md no rule holds, got:\n%s", out)
	}
}
