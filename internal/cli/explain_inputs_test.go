package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// explain --inputs lists what a hook must watch: the config, the source
// tree, .gitignore, a source directory outside .agnostic-ai/, and each
// file a review inlines with @path, but not an @path inside a code fence.
func TestExplainInputs_ListsConfigSourcesAndReviewIncludes(t *testing.T) {
	dir := setupFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [cursor]\nsources:\n  rules: docs/rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apps", "web", "README.md"), "Web notes.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "web.md"), "---\nname: web\n---\n@apps/web/README.md\n\n```\n@not/this.md\n```\n")
	testutil.Chdir(t, dir)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "--inputs"})
	if err := root.Execute(); err != nil {
		t.Fatalf("explain --inputs: %v", err)
	}
	want := ".agnostic-ai/**\n.gitignore\nagnostic-ai.local.yaml\nagnostic-ai.yaml\napps/web/README.md\ndocs/rules/**\n"
	if got := out.String(); got != want {
		t.Errorf("explain --inputs:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(out.String(), "not/this.md") {
		t.Error("an @path inside a code fence is not an input")
	}
}
