package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const externalInstructions = "external-only instructions"

// projectWithExternalEntryFile creates a project holding toolDir and an
// entry file name that links to a file outside the project, and enters it.
func projectWithExternalEntryFile(t *testing.T, toolDir, name string) string {
	t.Helper()
	external := filepath.Join(t.TempDir(), name)
	must(t, os.WriteFile(external, []byte("# Shared\n\n"+externalInstructions+"\n"), 0o644))
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, toolDir), 0o755))
	if err := os.Symlink(external, filepath.Join(dir, name)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	testutil.Chdir(t, dir)
	return dir
}

func TestImportAll_SkipsClaudeEntryFileLinkedOutsideTheProject(t *testing.T) {
	dir := projectWithExternalEntryFile(t, ".claude", "CLAUDE.md")

	runCmd(t, "import", "all")

	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai"), externalInstructions)
}

func TestImportAll_SkipsGeminiEntryFileLinkedOutsideTheProject(t *testing.T) {
	dir := projectWithExternalEntryFile(t, ".gemini", "GEMINI.md")

	runCmd(t, "import", "all")

	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai"), externalInstructions)
}

func TestImportClaude_FollowsEntryFileLinkedOutsideTheProject(t *testing.T) {
	dir := projectWithExternalEntryFile(t, ".claude", "CLAUDE.md")

	runCmd(t, "import", "claude")

	assertContains(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), externalInstructions)
}
