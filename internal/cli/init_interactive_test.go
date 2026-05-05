package cli

import (
	"errors"
	"io"
	"os"
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

func TestSelectTargets_PipedReader(t *testing.T) {
	got, err := selectTargets(strings.NewReader("claude,codex\n"), io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"claude", "codex"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSelectTargets_NonTTYNoData(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close() // immediate EOF: not a TTY, no data
	t.Cleanup(func() { _ = r.Close() })

	_, err = selectTargets(r, io.Discard)
	if err == nil {
		t.Fatal("expected error when stdin is neither a TTY nor a usable pipe")
	}
	if !strings.Contains(err.Error(), "interactive terminal") {
		t.Errorf("error should mention interactive terminal, got: %v", err)
	}
}

func TestSelectTargets_PipedEmptyReturnsErrNoTargets(t *testing.T) {
	_, err := selectTargets(strings.NewReader("\n"), io.Discard)
	if !errors.Is(err, errNoTargets) {
		t.Errorf("want errNoTargets, got %v", err)
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
