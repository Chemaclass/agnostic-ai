package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setupDroppedFixture writes a project with a rule (cursor-supported) and a
// settings spec (cursor-unsupported, so it buffers a capability warning the
// per-target summary can report). droppedSummary toggles the opt-in flag.
func setupDroppedFixture(t *testing.T, droppedSummary bool) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg := "version: 1\ntargets:\n  - cursor\n"
	if droppedSummary {
		cfg += "sync:\n  dropped-summary: true\n"
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "rules"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"),
		[]byte("---\nname: r1\n---\nrule body"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "settings"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "settings", "defaults.yaml"),
		[]byte("model: claude-opus-4-8\n"), 0o644))
	return dir
}

// captureLog redirects the package log sink to a buffer at default
// verbosity for the duration of the test.
func captureLog(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prevOut, prevV := logOut, verbosity
	logOut = buf
	verbosity = levelDefault
	t.Cleanup(func() { logOut, verbosity = prevOut, prevV })
	return buf
}

func TestSync_DroppedSummary_RendersWhenEnabled(t *testing.T) {
	dir := setupDroppedFixture(t, true)
	testutil.Chdir(t, dir)
	silence(t)
	buf := captureLog(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "dropped summary (per target):") {
		t.Errorf("expected per-target summary, got:\n%s", out)
	}
	if !strings.Contains(out, "cursor: 1 settings dropped (unsupported)") {
		t.Errorf("expected cursor settings drop, got:\n%s", out)
	}
}

func TestSync_DroppedSummary_SilentWhenDisabled(t *testing.T) {
	dir := setupDroppedFixture(t, false)
	testutil.Chdir(t, dir)
	silence(t)
	buf := captureLog(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Contains(buf.String(), "dropped summary") {
		t.Errorf("summary must stay silent when the flag is off, got:\n%s", buf.String())
	}
}
