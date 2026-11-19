package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplace_ReplacesExistingFileWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte("a much longer old body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("path = %q", got)
	}
}

func TestReplace_SetsExactModeOverExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits differ on Windows")
	}
	path := filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new"), 0o644); err != nil {
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

func TestReplace_ReplacesSymlinkWithRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Replace(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("link mode = %v, want a regular file", info.Mode())
	}
	if got, _ := os.ReadFile(link); string(got) != "new" {
		t.Errorf("link = %q", got)
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Errorf("symlink target was written: %q", got)
	}
}

func TestReplace_FailedRenameLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new"), 0o644); err == nil {
		t.Fatal("want an error replacing a non-empty directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}
