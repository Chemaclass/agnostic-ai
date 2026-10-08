package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
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

	got, err := findUnmanagedConfig(dir, &config.Config{})
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

// import cursor reads BUGBOT.md at every scope (#1276), so doctor lists
// the nested files too, and skips a generated one.
func TestFindUnmanagedConfig_FlagsNestedBugbotFiles(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".cursor", "BUGBOT.md"), "root review\n")
	mustWriteFile(t, filepath.Join(dir, "api", ".cursor", "BUGBOT.md"), "api review\n")
	mustWriteFile(t, filepath.Join(dir, "web", "ui", ".cursor", "BUGBOT.md"), "ui review\n")
	mustWriteFile(t, filepath.Join(dir, "docs", ".cursor", "BUGBOT.md"), "<!-- "+header.Marker+" -->\ngenerated\n")

	got, err := findUnmanagedConfig(dir, &config.Config{})
	if err != nil {
		t.Fatalf("findUnmanagedConfig: %v", err)
	}
	want := []unmanagedFinding{
		{Path: ".cursor/BUGBOT.md", Target: "cursor"},
		{Path: "api/.cursor/BUGBOT.md", Target: "cursor"},
		{Path: "web/ui/.cursor/BUGBOT.md", Target: "cursor"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %+v, want %+v", got, want)
	}
}

// A clean tree with no known config files yields no findings.
func TestFindUnmanagedConfig_EmptyTree(t *testing.T) {
	got, err := findUnmanagedConfig(t.TempDir(), &config.Config{})
	if err != nil {
		t.Fatalf("findUnmanagedConfig: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no findings in empty tree, got %+v", got)
	}
}

// A directory a config glob matches is not a config file.
func TestFindUnmanagedConfig_SkipsADirectoryAGlobMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "agents", "nested.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findUnmanagedConfig(dir, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a directory must not be reported: %+v", got)
	}
}
