package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportWindsurf_NestedGlobsRoundTrip(t *testing.T) {
	packageDir, err := os.Getwd()
	must(t, err)
	binary := builtBinary(t)
	project := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [windsurf]\n",
		"backend/.devin/rules/style.md": "---\ntrigger: glob\nglobs: ['*.go']\n---\n\nGo style.\n",
	} {
		path := filepath.Join(project, name)
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	for _, args := range [][]string{
		{"import", "windsurf"},
		{"sync", "--gitignore=off"},
		{"sync", "--check", "--gitignore=off"},
		{"lint"},
		{"import", "windsurf", "--overwrite"},
		{"sync", "--check", "--gitignore=off"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".devin/rules/style.md")); !os.IsNotExist(err) {
		t.Errorf("unexpected root rule: %v", err)
	}
	assertContains(t, filepath.Join(project, ".agnostic-ai/rules/backend/style.md"), "x-windsurf:", "- '*.go'")
	testutil.AssertGoldenTree(t, project, filepath.Join(packageDir, "fixtures", "windsurf-nested-globs"), "agnostic-ai.yaml")
}
