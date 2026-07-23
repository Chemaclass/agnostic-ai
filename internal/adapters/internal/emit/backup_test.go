package emit

import (
	"os"
	"path/filepath"
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
