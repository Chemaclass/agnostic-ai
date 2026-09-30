package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRuleHeadingContext_SyncNestsRuleSections(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":                    "version: 1\ntargets: [codex, gemini]\n",
		".agnostic-ai/AGNOSTIC_AI.md":         "# Project\n\nRoot guidance.\n",
		".agnostic-ai/rules/content.md":       "---\nname: content\n---\n### Doc versioning\n\nVersioning text.\n\n#### Version details\n\n~~~markdown\n### Code heading\n~~~\n",
		".agnostic-ai/rules/working-style.md": "---\nname: working-style\n---\nStyle text.\n",
	} {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0644))
	}
	for _, args := range [][]string{{"sync", "--gitignore=off"}, {"sync", "--check", "--gitignore=off"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	for _, path := range []string{"AGENTS.md", "GEMINI.md"} {
		assertContains(t, filepath.Join(dir, path), "### content\n", "#### Doc versioning\n", "##### Version details\n", "~~~markdown\n### Code heading\n~~~", "### working-style\n")
		assertNoFileContains(t, filepath.Join(dir, path), "\n### Doc versioning\n")
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "fixtures", "rule-heading-context"), "agnostic-ai.yaml")
}
