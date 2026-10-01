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
