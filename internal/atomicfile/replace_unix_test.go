//go:build unix

package atomicfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReplace_IgnoresUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	path := filepath.Join(t.TempDir(), "fresh")
	if err := Replace(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}
