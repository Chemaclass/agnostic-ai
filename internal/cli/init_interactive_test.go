package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

// TestDetectExistingTargets_GooseWithoutHintsFile covers #906. Goose's
// only marker was `.goosehints`, which sync writes only when the
// `outputs.goose.rules-file` opt-in is set. A project agnostic-ai had
// synced for goose with the default config therefore carried no goose
// marker at all, so `import all` skipped the target every time.
func TestDetectExistingTargets_GooseWithoutHintsFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		create func(t *testing.T, dir string)
	}{
		{"review file", func(t *testing.T, dir string) {
			mustWriteFile(t, filepath.Join(dir, ".agents", "REVIEW.md"), "# review\n")
		}},
		{"plugin package", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".agents", "plugins", "agnostic-ai"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.create(t, dir)
			if got := detectExistingTargets(dir); !slices.Contains(got, "goose") {
				t.Errorf("goose not detected from its %s, got %v", tc.name, got)
			}
		})
	}
}

// TestDetectExistingTargets_GooseMarkersDoNotClaimNeighbors guards the
// narrow-marker choice behind #906. `.agents/` is a shared convention,
// so a marker under it must not make an openhands or antigravity
// project look like a goose one.
func TestDetectExistingTargets_GooseMarkersDoNotClaimNeighbors(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{".openhands", ".agent", filepath.Join(".agents", "rules")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	if got := detectExistingTargets(dir); slices.Contains(got, "goose") {
		t.Errorf("goose claimed an openhands/antigravity project: %v", got)
	}
}

// A project that only has a root CLAUDE.md or GEMINI.md already uses that
// CLI; each file is written by exactly one target, so it is an exclusive
// marker. AGENTS.md stays out: most of the registry reads it.
func TestDetectExistingTargets_RootEntryFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"CLAUDE.md", "GEMINI.md", "AGENTS.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("# x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	got := detectExistingTargets(dir)
	want := []string{"claude", "gemini"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The root entry-file markers never pull a file from outside the project
// into automatic detection (and so into `import all`): a symlink counts
// only when it resolves inside root, and the marker must be a file.
func TestDetectExistingTargets_RootEntryFileStaysInsideTheProject(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "CLAUDE.md")
	if err := os.WriteFile(secret, []byte("# elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if got := detectExistingTargets(dir); len(got) != 0 {
		t.Errorf("a CLAUDE.md linking outside the project must not be detected, got %v", got)
	}

	inside := t.TempDir()
	if err := os.WriteFile(filepath.Join(inside, "docs.md"), []byte("# here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("docs.md", filepath.Join(inside, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	if got := detectExistingTargets(inside); !equalStrings(got, []string{"claude"}) {
		t.Errorf("a CLAUDE.md linking inside the project is detected, got %v", got)
	}

	wrongType := t.TempDir()
	if err := os.Mkdir(filepath.Join(wrongType, "GEMINI.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := detectExistingTargets(wrongType); len(got) != 0 {
		t.Errorf("a directory named GEMINI.md is not a marker, got %v", got)
	}
}

// Production callers pass root ".". An absolute link that stays inside
// the project must still count.
func TestDetectExistingTargets_AbsoluteInProjectLinkFromDotRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	target := filepath.Join(dir, "docs", "claude.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, "CLAUDE.md"); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if got := detectExistingTargets("."); !equalStrings(got, []string{"claude"}) {
		t.Errorf("absolute in-project link from root \".\": got %v", got)
	}
}
