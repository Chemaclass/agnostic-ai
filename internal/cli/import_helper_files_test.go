package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCaptureHelperFiles_Claude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), "project memory body\n")
	writeFile(t, filepath.Join(dir, ".claude/README.md"), "operator docs\n")
	scriptPath := filepath.Join(dir, ".claude/statusline.sh")
	writeFile(t, scriptPath, "#!/bin/sh\necho prompt\n")
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatal(err)
	}

	captured, err := captureHelperFiles(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 3 {
		t.Errorf("expected 3 captured, got %v", captured)
	}

	for _, name := range []string{"CLAUDE.md", "README.md", "statusline.sh"} {
		path := filepath.Join(dir, ".agnostic-ai/overlays/claude", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing overlay %s: %v", name, err)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ".agnostic-ai/overlays/claude/statusline.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("captured statusline.sh lost exec bit: mode=%v", info.Mode().Perm())
		}
	}
}

// Missing helpers are silent — most projects do not ship every helper.
func TestCaptureHelperFiles_NoHelpers(t *testing.T) {
	dir := t.TempDir()
	captured, err := captureHelperFiles(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 0 {
		t.Errorf("expected empty list, got %v", captured)
	}
}

// importFromClaude → claude adapter Emit round-trips the helpers
// without losing CLAUDE.md, README.md, statusline.sh (and its exec bit).
func TestImportClaudeThenSync_HelperFilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), "memory body\n")
	writeFile(t, filepath.Join(dir, ".claude/README.md"), "docs\n")
	scriptPath := filepath.Join(dir, ".claude/statusline.sh")
	writeFile(t, scriptPath, "#!/bin/sh\necho hi\n")
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Project\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	// Wipe the native dir to prove the round-trip rebuilds from overlay.
	if err := os.RemoveAll(filepath.Join(dir, ".claude")); err != nil {
		t.Fatal(err)
	}
	// Recreate empty .claude/ so sync writes into a fresh tree without
	// hitting the agnostic-managed entrypoint overwrite warning.
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeMinimalConfig(t, dir, ".agnostic-ai")
	prevDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	silence(t)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []struct {
		name string
		body string
	}{
		{"CLAUDE.md", "memory body\n"},
		{"README.md", "docs\n"},
		{"statusline.sh", "#!/bin/sh\necho hi\n"},
	} {
		got, err := os.ReadFile(filepath.Join(dir, ".claude", want.name))
		if err != nil {
			t.Errorf("restored %s missing: %v", want.name, err)
			continue
		}
		if string(got) != want.body {
			t.Errorf("%s body drift: got %q want %q", want.name, got, want.body)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ".claude/statusline.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("restored statusline.sh lost exec bit: mode=%v", info.Mode().Perm())
		}
	}
}
