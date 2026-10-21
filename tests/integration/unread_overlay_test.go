package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An overlay no tool reads fails `lint --strict`, and the route the
// finding names, `x-gemini` in a settings spec, reaches Gemini CLI (#2367).
func TestLint_UnreadGeminiOverlayFailsStrictUntilMovedToXGemini(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	overlay := filepath.Join(dir, ".agnostic-ai", "overlays", "gemini.settings.json")
	files := map[string]string{
		"agnostic-ai.yaml":                           "version: 1\ntargets: [claude, gemini]\n",
		".agnostic-ai/overlays/gemini.settings.json": `{"ui": {"theme": "Dracula"}}` + "\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "sync")
	runCmdExpectErr(t, "lint", "--strict")

	must(t, os.Remove(overlay))
	settings := filepath.Join(dir, ".agnostic-ai", "settings", "gemini.yaml")
	must(t, os.MkdirAll(filepath.Dir(settings), 0o755))
	must(t, os.WriteFile(settings, []byte("name: gemini\nx-gemini:\n  ui:\n    theme: Dracula\n"), 0o644))

	runCmd(t, "sync")
	runCmd(t, "lint", "--strict")
	assertContains(t, filepath.Join(dir, ".gemini", "settings.json"), "Dracula")
}
