package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSync_Check_PassesWhenInSync(t *testing.T) {
	dir := setupFixture(t)
	chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	if err := root.Execute(); err != nil {
		t.Errorf("check should pass after sync, got: %v", err)
	}
}

func TestDoctor_DetectsMissing(t *testing.T) {
	dir := setupFixture(t)
	chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	err := root.Execute()
	if err == nil {
		t.Error("doctor should fail when no sync has been run")
	}
}

func TestDoctor_DetectsStale(t *testing.T) {
	dir := setupFixture(t)
	chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err == nil {
		t.Error("doctor should detect stale CLAUDE.md")
	}
}
