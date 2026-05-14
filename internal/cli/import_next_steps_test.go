package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintImportNextSteps_PrintsSyncBlock(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	out := buf.String()
	for _, want := range []string{
		"next steps:",
		"agnostic-ai sync --check",
		"agnostic-ai sync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintImportNextSteps_FiltersJustImported(t *testing.T) {
	dir := t.TempDir()
	// Seed a claude marker. The hint for claude must not appear when the
	// user just imported claude.
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	if strings.Contains(buf.String(), "import claude") {
		t.Errorf("hint should not echo the just-imported target:\n%s", buf.String())
	}
}

func TestPrintImportNextSteps_SuggestsOtherDetectedTargets(t *testing.T) {
	dir := t.TempDir()
	// Seed a codex marker so importing claude points users at codex next.
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	out := buf.String()
	wantHint := "also detected codex/ - run 'agnostic-ai import codex' to import it"
	if !strings.Contains(out, wantHint) {
		t.Errorf("expected codex hint %q in output:\n%s", wantHint, out)
	}
}

func TestPrintImportNextSteps_TruncatesAtThreeAndCounts(t *testing.T) {
	dir := t.TempDir()
	// Drop markers for five extra targets so the cap (3) kicks in.
	markers := []string{".codex", ".gemini", ".cursor", ".amp", ".zed"}
	for _, m := range markers {
		if err := os.MkdirAll(filepath.Join(dir, m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	out := buf.String()
	if !strings.Contains(out, "(and 2 more detected targets)") {
		t.Errorf("expected truncation footer:\n%s", out)
	}
}

func TestPrintImportNextSteps_NoDetectedTargetsOmitsHints(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	out := buf.String()
	if strings.Contains(out, "also detected") {
		t.Errorf("should not show hints when nothing else is detected:\n%s", out)
	}
}
