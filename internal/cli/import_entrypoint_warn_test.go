package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A project with a rich CLAUDE.md plus a distinct, hand-authored AGENTS.md
// must not lose the AGENTS.md content silently: import mirrors CLAUDE.md
// into the shared body and warns that AGENTS.md holds unique content sync
// would overwrite (#415).
func TestImportFromClaude_WarnsAboutDivergentSiblingEntryPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)

	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Claude guide\n\nFull instructions.\n")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Agents\n\nUnique codex-only notes.\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "AGENTS.md") || !strings.Contains(out, "unique content") {
		t.Errorf("expected a warning naming AGENTS.md, got:\n%s", out)
	}
	// The captured body still comes from CLAUDE.md.
	got := mustRead(t, filepath.Join(dir, agnosticMainFile))
	if !strings.Contains(got, "Full instructions.") {
		t.Errorf("AGNOSTIC_AI.md should hold the CLAUDE.md body, got:\n%s", got)
	}
}

// When the sibling entry-point holds the same content as the mirrored
// source (already in sync), no warning fires: sync would rewrite it with
// an identical body, so nothing is lost.
func TestImportFromClaude_NoWarnWhenSiblingMatches(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)

	body := "# Shared\n\nIdentical body.\n"
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), body)
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), body)

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "unique content") {
		t.Errorf("did not expect a divergence warning, got:\n%s", buf.String())
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
