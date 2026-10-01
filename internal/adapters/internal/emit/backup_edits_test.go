package emit

import (
	"os"
	"path/filepath"
	"testing"
)

// A rolled-back sync removes the backup it made, so a retry does not take
// it for an earlier one and leave the edit stuck in place.
func TestRollback_RemovesAHandEditBackupTheRunMade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSession()
	s.BackUpEditsSince(map[string]string{path: ContentSum("generated\n")})
	s.StartTransaction()
	if err := s.WriteFile(path, "generated twice\n", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("no backup before rollback: %v", err)
	}

	if err := s.Rollback(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("rollback kept the backup: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "hand edit\n" {
		t.Errorf("rollback left %q, want the hand edit", data)
	}
}

// A link's target may live outside the project, so sync never copies it
// into a backup.
func TestBackUpEdits_NeverCopiesALinkTarget(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.Symlink(outside, path); err != nil {
		t.Skip(err)
	}
	s := NewSession()
	s.BackUpEditsSince(map[string]string{path: ContentSum("generated\n")})

	_ = s.WriteFile(path, "generated twice\n", false)

	if _, err := os.Lstat(path + ".bak"); err == nil {
		t.Error("sync copied a link target into a backup")
	}
}
