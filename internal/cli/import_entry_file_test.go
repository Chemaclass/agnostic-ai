package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEntryFile_ImportAllNotesEachSkippedFileOnce(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(outside, []byte("# elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	link := filepath.Join(root, "CLAUDE.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	var out bytes.Buffer
	prevOut := logOut
	logOut = &out
	importAllSkippedEntryFiles = map[string]bool{}
	t.Cleanup(func() {
		logOut = prevOut
		importAllSkippedEntryFiles = nil
	})

	for range 2 {
		if _, err := readEntryFile(root, link); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("want fs.ErrNotExist for a link outside the project, got %v", err)
		}
	}
	if got := strings.Count(out.String(), "skipped "+link); got != 1 {
		t.Errorf("noted the skipped file %d times, want once:\n%s", got, out.String())
	}
}
