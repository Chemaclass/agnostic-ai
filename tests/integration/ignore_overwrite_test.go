package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// ignoreTargets pairs each target that emits an ignore file with the
// path it writes. Kept next to the test that needs it rather than
// imported from internal/cli, so a rename on either side shows up here
// as a real failure instead of silently tracking the emitter.
var ignoreTargets = map[string]string{
	"aider":    ".aiderignore",
	"cursor":   ".cursorignore",
	"gemini":   ".geminiignore",
	"junie":    ".aiignore",
	"kiro":     ".kiroignore",
	"trae":     ".trae/.ignore",
	"windsurf": ".devinignore",
}

// TestIgnore_HandAuthoredFileSurvivesSync covers the case that had no
// coverage at all before #754, which is why a silent overwrite shipped:
// a repo holding both a hand-authored ignore file and an ignore spec.
// Every one of the seven targets must refuse to destroy the file, then
// carry both sets of patterns once `import` has rescued it.
func TestIgnore_HandAuthoredFileSurvivesSync(t *testing.T) {
	for target, ignoreFile := range ignoreTargets {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)

			writeIgnoreFixture(t, dir, target, ignoreFile)

			root := cli.NewRootCmd("test")
			root.SetArgs([]string{"sync", "-t", target})
			if err := root.Execute(); err == nil {
				t.Fatal("sync must refuse to overwrite a hand-authored ignore file")
			}
			assertContains(t, filepath.Join(dir, filepath.FromSlash(ignoreFile)),
				"my-secrets/", "*.key")

			runCmd(t, "import", target)
			runCmd(t, "sync", "-t", target)

			assertContains(t, filepath.Join(dir, filepath.FromSlash(ignoreFile)),
				"my-secrets/", "*.key", "dist/", "node_modules/")
		})
	}
}

// TestIgnore_HandAuthoredFileSurvivesWithNoSpec is the control: with no
// ignore spec declared, the adapter writes no ignore file at all and
// the hand-authored one is left exactly as it was.
func TestIgnore_HandAuthoredFileSurvivesWithNoSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	const handAuthored = "# hand-authored by the team\nmy-secrets/\n*.key\n"
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	mustWrite(t, filepath.Join(dir, ".kiroignore"), handAuthored)

	runCmd(t, "sync", "-t", "kiro")

	got, err := os.ReadFile(filepath.Join(dir, ".kiroignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != handAuthored {
		t.Errorf("a repo with no ignore spec must leave the file alone, got:\n%s", got)
	}
}

func writeIgnoreFixture(t *testing.T, dir, target, ignoreFile string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "ignore", "build.md"),
		"---\nname: build\n---\n\ndist/\nnode_modules/\n")
	mustWrite(t, filepath.Join(dir, filepath.FromSlash(ignoreFile)),
		"# hand-authored by the team\nmy-secrets/\n*.key\n")
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
