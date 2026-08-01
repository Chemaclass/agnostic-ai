package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePipedSelection_Names(t *testing.T) {
	got, err := parsePipedSelection("claude,codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"claude", "codex"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParsePipedSelection_TrimsAndDedupes(t *testing.T) {
	got, err := parsePipedSelection(" claude , codex , claude ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"claude", "codex"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParsePipedSelection_PreservesCanonicalOrder(t *testing.T) {
	// Input order is reversed; output must follow allTargets order.
	got, err := parsePipedSelection("codex,claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"claude", "codex"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParsePipedSelection_RejectsUnknown(t *testing.T) {
	_, err := parsePipedSelection("claude,fnord")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "fnord") {
		t.Errorf("error should mention unknown name, got: %v", err)
	}
}

func TestParsePipedSelection_Empty(t *testing.T) {
	for _, in := range []string{"", "\n", "  ", " , , "} {
		_, err := parsePipedSelection(in)
		if !errors.Is(err, errNoTargets) {
			t.Errorf("input %q: want errNoTargets, got %v", in, err)
		}
	}
}

func TestDetectExistingTargets_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := detectExistingTargets(dir)
	if len(got) != 0 {
		t.Errorf("empty dir: want no targets, got %v", got)
	}
}

func TestDetectExistingTargets_DirMarkers(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{".claude", ".codex", ".gemini", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	got := detectExistingTargets(dir)
	want := []string{"claude", "codex", "gemini", "cursor"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectExistingTargets_CopilotFileMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".github", "copilot-instructions.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := detectExistingTargets(dir)
	want := []string{"copilot"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectExistingTargets_AgentsAgentsTriggersCodex(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "agents"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := detectExistingTargets(dir)
	want := []string{"codex"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectExistingTargets_AgentsRulesTriggersAntigravity(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := detectExistingTargets(dir)
	want := []string{"antigravity"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectExistingTargets_LegacyAgentTriggersAntigravity(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agent", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := detectExistingTargets(dir)
	want := []string{"antigravity"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The .augment directory now holds real native output
// (.augment/rules/, .augment/agents/) rather than only the opt-in
// .augment-guidelines file, so it must detect an existing Augment
// project on its own (target-audit 2026-08-01).
func TestDetectExistingTargets_AugmentDirMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".augment", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := detectExistingTargets(dir)
	want := []string{"augment"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectExistingTargets_CanonicalOrder(t *testing.T) {
	dir := t.TempDir()
	// Create in non-canonical order; result must still follow allTargets.
	for _, sub := range []string{".opencode", ".claude", ".zed", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	got := detectExistingTargets(dir)
	want := []string{"claude", "cursor", "zed", "opencode"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
