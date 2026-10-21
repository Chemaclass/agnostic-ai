package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedOverlayProject(t *testing.T, overlays map[string]string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, gemini]\ngitignore:\n  enabled: false\n")
	for name, body := range overlays {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "overlays", filepath.FromSlash(name)), body)
	}
	silence(t)
}

func TestRunSyncOnce_NotesGeminiOverlayAndPointsToXGemini(t *testing.T) {
	seedOverlayProject(t, map[string]string{"gemini.settings.json": `{"ui": {"theme": "Dracula"}}`})
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	got := notes.String()
	if n := strings.Count(got, ".agnostic-ai/overlays/gemini.settings.json"); n != 1 {
		t.Fatalf("want one note naming the overlay, got %d:\n%s", n, got)
	}
	for _, want := range []string{"sync does not read", "x-gemini", "settings/ spec"} {
		if !strings.Contains(got, want) {
			t.Errorf("note lacks %q:\n%s", want, got)
		}
	}
}

func TestRunSyncOnce_NotesMisspelledOverlayAndListsKnownOnes(t *testing.T) {
	seedOverlayProject(t, map[string]string{"claude.setings.json": `{"statusLine": {}}`})
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	got := notes.String()
	for _, want := range []string{".agnostic-ai/overlays/claude.setings.json", "did you mean claude.settings.json", "codex.config.toml", "claude/<file>"} {
		if !strings.Contains(got, want) {
			t.Errorf("note lacks %q:\n%s", want, got)
		}
	}
}

func TestRunSyncOnce_KnownOverlaysRaiseNoNote(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		"claude.settings.json":             "{}\n",
		"claude.settings.hook-events.json": "{}\n",
		"codex.config.toml":                "model = \"o3\"\n",
		"codex.exec-policies.yaml":         "[]\n",
		"codex.exec-policies-header.txt":   "Header.\n",
		"claude/statusline.sh":             "#!/bin/sh\n",
		"claude/launch.json":               "{}\n",
		"codex/README.md":                  "# Codex\n",
		".DS_Store":                        "",
	})
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(notes.String(), "overlays/") {
		t.Errorf("known overlays raised a note:\n%s", notes.String())
	}
}

func TestLint_WarnsAboutOverlaysSyncDoesNotRead(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		"gemini.settings.json": "{}\n",
		"claude/nested/x.md":   "x\n",
		"claude.settings.json": "{}\n",
	})

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT051")
	if len(lines) != 2 {
		t.Fatalf("want two LINT051 lines, got:\n%s", out)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"overlays/gemini.settings.json", "overlays/claude/nested/x.md"} {
		if !strings.Contains(joined, want) {
			t.Errorf("LINT051 lines lack %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "overlays/claude.settings.json:") {
		t.Errorf("a known overlay was flagged:\n%s", joined)
	}
}

// Every overlay an importer writes must be one sync reads, or the warning
// would flag a file `import` just created.
func TestUnreadOverlays_ImportedOverlaysAreKnown(t *testing.T) {
	root := "."
	paths := []string{
		claudeOverlayPath(root),
		codexOverlayPath(root),
		codexExecPoliciesOverlayPath(root),
		codexExecPoliciesHeaderOverlayPath(root),
		filepath.Join(root, agnosticOverlayDir, claudeHookOrderFile),
	}
	for tool, helpers := range helperFilesByTool {
		for _, h := range helpers {
			paths = append(paths, helperOverlayPath(root, tool, h.basename))
		}
	}
	for _, p := range paths {
		rel, err := filepath.Rel(agnosticOverlayDir, p)
		if err != nil {
			t.Fatal(err)
		}
		if !overlayKnown(filepath.ToSlash(rel)) {
			t.Errorf("%s is written by import but no target reads it", p)
		}
	}
}
