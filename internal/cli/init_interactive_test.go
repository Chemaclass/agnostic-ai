package cli

import (
	"errors"
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
