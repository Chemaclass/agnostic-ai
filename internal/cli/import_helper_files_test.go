package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// A project that keeps its instructions nested under .claude/CLAUDE.md
// (no root CLAUDE.md) has that body promoted to the shared AGNOSTIC_AI.md
// so `sync` distributes it to every target's entry-point — not buried in
// a claude-private overlay where codex/gemini never see it.
func TestImportClaude_NestedMainFilePromotedToSharedBody(t *testing.T) {
	dir := t.TempDir()
	body := "# phel-doom\n\nTerminal raycaster. Nested instructions body.\n"
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), body)
	writeFile(t, filepath.Join(dir, ".claude/README.md"), "operator docs\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	// Nested CLAUDE.md becomes the shared body.
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("AGNOSTIC_AI.md not seeded: %v", err)
	}
	if !strings.Contains(string(got), "Nested instructions body.") {
		t.Errorf("AGNOSTIC_AI.md missing nested body, got %q", got)
	}

	// It must NOT also be captured as a claude-private helper overlay
	// (that would restore a duplicate copy under .claude/ on sync).
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/overlays/claude/CLAUDE.md")); err == nil {
		t.Error("nested CLAUDE.md should not be captured as a claude-private overlay when promoted")
	}
	// Other helpers are still captured.
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/overlays/claude/README.md")); err != nil {
		t.Errorf("README.md helper should still be captured: %v", err)
	}
}

// End-to-end: nested-only .claude/CLAUDE.md propagates to both the claude
// (CLAUDE.md) and codex (AGENTS.md) entry-points after sync.
func TestImportClaudeNestedThenSync_PropagatesToCodex(t *testing.T) {
	dir := t.TempDir()
	body := "# phel-doom\n\nNested instructions body for every tool.\n"
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), body)

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	writeMinimalConfig(t, dir, ".agnostic-ai")

	prevDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,codex"})
	silence(t)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s entry-point missing: %v", name, err)
			continue
		}
		if !strings.Contains(string(got), "Nested instructions body for every tool.") {
			t.Errorf("%s missing shared body, got %q", name, got)
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
