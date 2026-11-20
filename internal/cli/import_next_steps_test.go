package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPrintImportNextSteps_PrintsSyncBlock(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	out := buf.String()
	for _, want := range []string{
		"next steps:",
		"agnostic-ai sync --plan",
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

func TestPrintImportNextSteps_AntigravityPluginProjectHintsNoGoose(t *testing.T) {
	dir := t.TempDir()
	plugin := filepath.Join(dir, ".agents", "plugins", "team")
	writeFile(t, filepath.Join(plugin, "plugin.json"), "{}\n")
	writeFile(t, filepath.Join(plugin, "mcp.json"), "{}\n")
	buf := captureSummary(t)
	printImportNextSteps(dir, "antigravity")
	if out := buf.String(); strings.Contains(out, "goose") {
		t.Errorf("antigravity plugin project got a goose hint:\n%s", out)
	}
}

func runImportForNextSteps(t *testing.T, config string, files map[string]string, args ...string) string {
	t.Helper()
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", config)
	for path, body := range files {
		writeFile(t, path, body)
	}
	buf := captureSummary(t)
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"import"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("import %v: %v", args, err)
	}
	return buf.String()
}

func TestImport_MultiSourceRunPrintsNextStepsOnce(t *testing.T) {
	files := map[string]string{
		".claude/rules/style.md":    "Use tabs.\n",
		".cursor/rules/review.mdc":  "---\ndescription: Review\n---\nReview the diff.\n",
		".windsurf/rules/naming.md": "Name things well.\n",
	}
	for name, args := range map[string][]string{
		"all":   {"all"},
		"named": {"claude", "cursor", "windsurf"},
	} {
		t.Run(name, func(t *testing.T) {
			out := runImportForNextSteps(t, "version: 1\n", files, args...)
			if got := strings.Count(out, "next steps:"); got != 1 {
				t.Errorf("want one next steps block, got %d:\n%s", got, out)
			}
			for _, imported := range []string{"claude", "cursor", "windsurf"} {
				if strings.Contains(out, "also detected "+imported+"/") {
					t.Errorf("suggests %s, which this run imported:\n%s", imported, out)
				}
			}
		})
	}
}

func TestImport_KimiAndVibePrintNextSteps(t *testing.T) {
	for _, source := range []string{"kimi", "vibe"} {
		t.Run(source, func(t *testing.T) {
			out := runImportForNextSteps(t, "version: 1\ntargets: ["+source+"]\n", nil, source)
			if got := strings.Count(out, "next steps:"); got != 1 {
				t.Errorf("want one next steps block, got %d:\n%s", got, out)
			}
		})
	}
}
