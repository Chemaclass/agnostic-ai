package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHoldStateFile_ServesOneCopyUntilReleased(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)

	release := holdStateFile(".")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Errorf("while held: sum %q, want the held one", got)
	}
	release()
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("after release: sum %q, want two from disk", got)
	}
}

func TestPriorStateFile_ReadsTheDiskWhenNothingIsHeld(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Fatalf("first read: sum %q, want one", got)
	}
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("same-size rewrite: sum %q, want two", got)
	}
}
