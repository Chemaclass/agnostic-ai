package integration

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestImportAllDryRun_LeavesOutEntryFileLinkedOutsideTheProject(t *testing.T) {
	projectWithExternalEntryFile(t, ".claude", "CLAUDE.md")

	out := captureStdout(t, func() { runCmd(t, "import", "all", "--dry-run", "--diff") })

	if strings.Contains(out, externalInstructions) {
		t.Errorf("import all --dry-run previewed the external entry file:\n%s", out)
	}
}

func TestImportClaudeDryRun_PreviewsEntryFileLinkedOutsideTheProject(t *testing.T) {
	projectWithExternalEntryFile(t, ".claude", "CLAUDE.md")

	out := captureStdout(t, func() { runCmd(t, "import", "claude", "--dry-run", "--diff") })

	if !strings.Contains(out, externalInstructions) {
		t.Errorf("import claude --dry-run should preview the linked entry file:\n%s", out)
	}
}

// captureStdout returns what fn writes to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	must(t, err)
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	return <-done
}
