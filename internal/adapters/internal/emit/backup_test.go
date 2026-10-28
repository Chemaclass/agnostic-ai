package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFile_BackupCreatesBakWhenContentDiffers(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)
	defer sess.SetBackup(false)

	if err := sess.WriteFile(path, "new", false); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != "old" {
		t.Errorf("expected .bak to hold old content, got %q", bak)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("expected new content (normalized newline), got %q", got)
	}
}

func TestWriteFile_BackupSkippedWhenContentEqual(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	// File on disk already ends with normalized trailing newline so the
	// no-op compare path matches what WriteFile normalizes the input to.
	if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)
	defer sess.SetBackup(false)

	if err := sess.WriteFile(path, "same", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected no .bak when content matches, err=%v", err)
	}
}

func TestWriteFile_BackupSkippedForNewFile(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.md")
	sess.SetBackup(true)
	defer sess.SetBackup(false)

	if err := sess.WriteFile(path, "content", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected no .bak when no prior file, err=%v", err)
	}
}

func TestWriteFile_BackupOffByDefault(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteFile(path, "new", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected no .bak with backup off, err=%v", err)
	}
}

func TestWriteFile_BackupKeepsTheSourceMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)

	if err := sess.WriteFile(path, "{}", false); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path+".bak", 0o600)
}

func TestRemoveCopy_BackupKeepsTheSourceMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)

	if _, err := sess.RemoveCopy(path, ContentSum("private"), false); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path+".bak", 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", filepath.Base(path), got, want)
	}
}

func TestWriteFile_BackupNarrowsAnEarlierWideBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("older"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)

	if err := sess.WriteFile(path, "{}", false); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path+".bak", 0o600)
}

func TestWriteFile_BackupRefusesALinkedBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path+".bak"); err != nil {
		t.Fatal(err)
	}
	sess.SetBackup(true)

	if err := sess.WriteFile(path, "new", false); err == nil {
		t.Error("backup wrote through a link")
	}
	if got, _ := os.ReadFile(outside); string(got) != "keep" {
		t.Errorf("link target = %q, want it untouched", got)
	}
}
