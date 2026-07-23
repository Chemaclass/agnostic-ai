package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransaction_RollbackRestoresOverwrittenFile(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.StartTransaction()
	if err := sess.WriteFile(path, "modified", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("expected original content after rollback, got %q", got)
	}
}

func TestTransaction_RollbackDeletesNewFile(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.md")

	sess.StartTransaction()
	if err := sess.WriteFile(path, "created", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be removed after rollback, stat err=%v", err)
	}
}

func TestTransaction_CommitPreservesWrites(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.StartTransaction()
	if err := sess.WriteFile(path, "new", false); err != nil {
		t.Fatal(err)
	}
	sess.Commit()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("expected new content after commit (normalized newline), got %q", got)
	}
}

func TestTransaction_RollbackMultipleFilesInReverseOrder(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")
	if err := os.WriteFile(fileA, []byte("a-old"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.StartTransaction()
	if err := sess.WriteFile(fileA, "a-new", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteFile(fileB, "b-created", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	gotA, err := os.ReadFile(fileA)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != "a-old" {
		t.Errorf("expected a-old after rollback, got %q", gotA)
	}
	if _, err := os.Stat(fileB); !os.IsNotExist(err) {
		t.Errorf("expected fileB removed after rollback, stat err=%v", err)
	}
}

func TestTransaction_RollbackNoOpsWithoutStart(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	if err := sess.Rollback(); err != nil {
		t.Errorf("rollback with no transaction should be a no-op, got %v", err)
	}
}

func TestTransaction_CommitClearsLog(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")

	sess.StartTransaction()
	if err := sess.WriteFile(path, "v1", false); err != nil {
		t.Fatal(err)
	}
	sess.Commit()

	// Second rollback should be a no-op (log was cleared by Commit).
	if err := sess.Rollback(); err != nil {
		t.Errorf("rollback after commit should be no-op, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1\n" {
		t.Errorf("file should still hold v1 after stale rollback (normalized newline), got %q", got)
	}
}

func TestTransaction_RollbackWithDetailedRecordingMode(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.md")
	brand := filepath.Join(dir, "brand-new.md")
	if err := os.WriteFile(existing, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.StartDetailedRecording()
	sess.StartTransaction()
	if err := sess.WriteFile(existing, "after", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteFile(brand, "fresh", false); err != nil {
		t.Fatal(err)
	}
	sess.StopDetailedRecording()
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before" {
		t.Errorf("expected before after rollback in detailing mode, got %q", got)
	}
	if _, err := os.Stat(brand); !os.IsNotExist(err) {
		t.Errorf("expected brand-new.md removed, stat err=%v", err)
	}
}
