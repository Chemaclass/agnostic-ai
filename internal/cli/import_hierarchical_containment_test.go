package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// A nested main file that links outside the project must not reach an
// import: `import all` runs this walk on detection alone, and the file it
// reads lands in the project's rule sources.
func TestFindHierarchicalMainFiles_SkipsLinksOutsideTheProject(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("token=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("# root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "sub", "GEMINI.md")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "in"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "GEMINI.md"), filepath.Join(root, "in", "GEMINI.md")); err != nil {
		t.Fatal(err)
	}

	got, err := findHierarchicalMainFiles(root, "GEMINI.md", config.Sources{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range got {
		rel, _ := filepath.Rel(root, f.path)
		paths = append(paths, filepath.ToSlash(rel))
	}
	want := []string{"GEMINI.md", "in/GEMINI.md"}
	if !equalStrings(paths, want) {
		t.Errorf("got %v, want %v (the outside link must be skipped)", paths, want)
	}
}
