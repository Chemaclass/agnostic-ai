package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// A hand-authored config file (no provenance marker) is reported as
// unmanaged; a generated sibling carrying the marker is not.
func TestFindUnmanagedConfig_FlagsOnlyUnmarkedFiles(t *testing.T) {
	dir := t.TempDir()
	// Hand-authored: no marker -> unmanaged.
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "# My project\n\nHand-written instructions.\n")
	mustWriteFile(t, filepath.Join(dir, ".cursor", "rules", "legacy.mdc"), "---\ndescription: x\n---\n\nold rule\n")
	// Generated: carries the marker -> managed, must not be flagged.
	mustWriteFile(t, filepath.Join(dir, "GEMINI.md"), "<!-- "+header.Marker+" -->\npointer body\n")

	got, err := findUnmanagedConfig(dir)
	if err != nil {
		t.Fatalf("findUnmanagedConfig: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unmanaged findings, got %d: %+v", len(got), got)
	}
	byPath := map[string]string{}
	for _, f := range got {
		byPath[f.Path] = f.Target
	}
	if byPath["CLAUDE.md"] != "claude" {
		t.Errorf("CLAUDE.md target = %q, want claude", byPath["CLAUDE.md"])
	}
	if byPath[".cursor/rules/legacy.mdc"] != "cursor" {
		t.Errorf("legacy.mdc target = %q, want cursor", byPath[".cursor/rules/legacy.mdc"])
	}
	if _, flagged := byPath["GEMINI.md"]; flagged {
		t.Errorf("generated GEMINI.md should not be flagged: %+v", got)
	}
}

// A clean tree with no known config files yields no findings.
func TestFindUnmanagedConfig_EmptyTree(t *testing.T) {
	got, err := findUnmanagedConfig(t.TempDir())
	if err != nil {
		t.Fatalf("findUnmanagedConfig: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no findings in empty tree, got %+v", got)
	}
}
