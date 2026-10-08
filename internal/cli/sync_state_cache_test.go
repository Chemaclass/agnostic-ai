package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCachedStateFile_ReadsTheFileAgainAfterItChanges(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)

	if got := cachedStateFile(".").OutputSums["a.md"]; got != "one" {
		t.Fatalf("first read: sum %q, want one", got)
	}
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two","b.md":"x"}}`)
	if got := cachedStateFile(".").OutputSums["a.md"]; got != "two" {
		t.Errorf("after a rewrite: sum %q, want two", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := cachedStateFile(".").OutputSums["a.md"]; got != "" {
		t.Errorf("after removal: sum %q, want none", got)
	}
}
