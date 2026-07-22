package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRestoreHelperFiles_IsRecorded guards the gitignore/ledger leak:
// helper files restored from an overlay must flow through the recorded
// write path so a generated file like `.claude/README.md` is captured by
// recording (and therefore added to the managed .gitignore block) rather
// than written silently via raw os.WriteFile.
func TestRestoreHelperFiles_IsRecorded(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	chdirHelper(t, dir)

	overlay := filepath.Join(agnosticOverlayDir, "claude")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "README.md"), []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.StartRecording()
	if err := sess.RestoreHelperFiles("claude", false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	paths := sess.StopRecording()

	want := filepath.Join(".claude", "README.md")
	found := false
	for _, p := range paths {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Errorf("restored helper not recorded; recorded=%v want %q", paths, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("helper file not written to disk: %v", err)
	}
}

// TestRestoreHelperFiles_PreservesMode keeps an executable overlay helper
// (e.g. a hook script) executable after restore.
func TestRestoreHelperFiles_PreservesMode(t *testing.T) {
	sess := NewSession()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support the unix executable bit")
	}
	dir := t.TempDir()
	chdirHelper(t, dir)

	overlay := filepath.Join(agnosticOverlayDir, "claude")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := sess.RestoreHelperFiles("claude", false); err != nil {
		t.Fatalf("restore: %v", err)
	}

	info, err := os.Stat(filepath.Join(".claude", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("executable bit lost on restore, mode=%v", info.Mode())
	}
}

func chdirHelper(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}
