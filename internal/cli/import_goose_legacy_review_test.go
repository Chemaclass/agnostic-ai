package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportGoose_LegacyReviewRemainsAvailableWithoutEmission(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	t.Setenv("GOOSE_PATH_ROOT", t.TempDir())
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [goose]\noutputs:\n  goose:\n    review-file: custom/REVIEW.md\n")
	const body = "Check changed code for compatibility.\n"
	for _, path := range []string{".agents/REVIEW.md", "api/.agents/REVIEW.md", "custom/REVIEW.md"} {
		writeFile(t, filepath.Join(dir, path), body)
	}
	execCLI(t, "import", "goose")
	reviewPath := filepath.Join(dir, ".agnostic-ai", "reviews", "review.md")
	imported := readFile(t, reviewPath)
	if !strings.Contains(imported, strings.TrimSpace(body)) {
		t.Errorf("legacy review was not imported: %q", imported)
	}
	execCLI(t, "sync", "-t", "goose")
	execCLI(t, "sync", "--check", "-t", "goose")
	execCLI(t, "lint")
	if got := readFile(t, reviewPath); got != imported {
		t.Errorf("portable Review spec changed: %q", got)
	}
	for _, path := range []string{".agents/REVIEW.md", "api/.agents/REVIEW.md", "custom/REVIEW.md"} {
		if got := readFile(t, filepath.Join(dir, path)); got != body {
			t.Errorf("user file %s changed: %q", path, got)
		}
	}
}
