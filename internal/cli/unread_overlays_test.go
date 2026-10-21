package cli

import (
	"os"
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

func overlayMessages(t *testing.T) map[string]string {
	t.Helper()
	unread, err := unreadOverlays(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, u := range unread {
		out[u.Path] = u.Message
	}
	return out
}

func TestUnreadOverlays_FollowsASymlinkedOverlaysFolder(t *testing.T) {
	seedOverlayProject(t, nil)
	real := t.TempDir()
	mustWriteFile(t, filepath.Join(real, "claude.settings.json"), "{\"statusLine\": {\"type\": \"command\", \"command\": \"x\"}}\n")
	mustWriteFile(t, filepath.Join(real, "gemini.settings.json"), "{}\n")
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(".agnostic-ai", "overlays")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	got := overlayMessages(t)
	if len(got) != 1 || got[".agnostic-ai/overlays/gemini.settings.json"] == "" {
		t.Fatalf("want only gemini.settings.json flagged, got %v", got)
	}
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(".claude", "settings.json")); err != nil || !strings.Contains(string(data), "statusLine") {
		t.Errorf("sync dropped the linked overlay: %v\n%s", err, data)
	}
}

func TestUnreadOverlays_OverlaysPathAsPlainFileFlagsNothing(t *testing.T) {
	seedOverlayProject(t, nil)
	mustWriteFile(t, filepath.Join(".agnostic-ai", "overlays"), "not a folder\n")
	if got := overlayMessages(t); len(got) != 0 {
		t.Errorf("want nothing flagged, got %v", got)
	}
	if out, _ := runCLI(t, "lint"); strings.Contains(out, "panic") {
		t.Errorf("lint panicked:\n%s", out)
	}
}

func TestUnreadOverlays_SkipsDotFoldersAndRootReadmes(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		".git/config":          "x\n",
		".archive/old.json":    "{}\n",
		"README.md":            "# Overlays\n",
		"readme.txt":           "notes\n",
		"claude/nested/README": "x\n",
	})
	got := overlayMessages(t)
	if len(got) != 1 || got[".agnostic-ai/overlays/claude/nested/README"] == "" {
		t.Errorf("want only the nested helper flagged, got %v", got)
	}
}

func TestUnreadOverlays_SymlinkedHelperFolderIsKnown(t *testing.T) {
	seedOverlayProject(t, nil)
	real := t.TempDir()
	mustWriteFile(t, filepath.Join(real, "README.md"), "# Claude\n")
	if err := os.MkdirAll(filepath.Join(".agnostic-ai", "overlays"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(".agnostic-ai", "overlays", "claude")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if got := overlayMessages(t); len(got) != 0 {
		t.Errorf("want nothing flagged, got %v", got)
	}
}

func TestUnreadOverlays_XTargetHintOnlyForSettingsFilesOfPassThroughTargets(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		"gemini.settings.json": "{}\n",
		"cursor.settings.json": "{}\n",
		"gemini/GEMINI.md":     "x\n",
		"cursor.mcp.json":      "{}\n",
	})
	got := overlayMessages(t)
	if !strings.Contains(got[".agnostic-ai/overlays/gemini.settings.json"], "x-gemini") {
		t.Errorf("gemini.settings.json lacks the x-gemini hint: %q", got[".agnostic-ai/overlays/gemini.settings.json"])
	}
	for _, p := range []string{"cursor.settings.json", "gemini/GEMINI.md", "cursor.mcp.json"} {
		msg := got[".agnostic-ai/overlays/"+p]
		if strings.Contains(msg, "x-") || !strings.Contains(msg, "codex.config.toml") {
			t.Errorf("%s: want the generic list, got %q", p, msg)
		}
	}
}

// settingsPassThroughTargets must name every adapter that merges a
// settings spec's x-<target> block into its own file.
func TestSettingsPassThroughTargets_MatchAdapters(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "adapters", "*"))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "emit.MergeSettingsCustomKeys(") {
				found[filepath.Base(d)] = true
			}
		}
	}
	for target := range found {
		if _, ok := settingsPassThroughTargets[target]; !ok {
			t.Errorf("%s merges x-%s settings but is missing from settingsPassThroughTargets", target, target)
		}
	}
	for target := range settingsPassThroughTargets {
		if !found[target] {
			t.Errorf("%s is in settingsPassThroughTargets but merges no x-%s settings", target, target)
		}
	}
}

func TestUnreadOverlays_CaseOnlyMatchSaysToRename(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		"Claude.settings.json": "{}\n",
		"Claude/README.md":     "x\n",
	})
	_, err := os.Stat(filepath.Join(".agnostic-ai", "overlays", "claude.settings.json"))
	insensitive := err == nil
	got := overlayMessages(t)
	for p, want := range map[string]string{"Claude.settings.json": "claude.settings.json", "Claude/README.md": "claude/README.md"} {
		msg := got[".agnostic-ai/overlays/"+p]
		if insensitive && (!strings.Contains(msg, "ignore case") || !strings.Contains(msg, "rename it to "+want)) {
			t.Errorf("%s: want a rename hint on a case-insensitive filesystem, got %q", p, msg)
		}
		if !insensitive && !strings.Contains(msg, "sync does not read") {
			t.Errorf("%s: want the not-read message, got %q", p, msg)
		}
	}
}

func TestUnreadOverlays_SuggestsHelperFolderForTypos(t *testing.T) {
	seedOverlayProject(t, map[string]string{
		"claud/README.md": "x\n",
		"codx/README.md":  "x\n",
	})
	got := overlayMessages(t)
	if msg := got[".agnostic-ai/overlays/claud/README.md"]; !strings.Contains(msg, "did you mean claude/?") {
		t.Errorf("claud: %q", msg)
	}
	if msg := got[".agnostic-ai/overlays/codx/README.md"]; !strings.Contains(msg, "did you mean codex/?") {
		t.Errorf("codx: %q", msg)
	}
}
