package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// hierarchicalGeminiWithOutsideLink builds a project whose sub/GEMINI.md
// links outside it and whose in/GEMINI.md links to the root copy.
func hierarchicalGeminiWithOutsideLink(t *testing.T) string {
	t.Helper()
	secret := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(secret, []byte("token=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("# root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"sub", "in"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(secret, filepath.Join(root, "sub", "GEMINI.md")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "GEMINI.md"), filepath.Join(root, "in", "GEMINI.md")); err != nil {
		t.Fatal(err)
	}
	return root
}

// rulesHolding reports whether any rule written to dir holds substr.
func rulesHolding(t *testing.T, dir, substr string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), substr) {
			return true
		}
	}
	return false
}

// `import all` runs the walk on detection alone, so a nested main file
// that links outside the project must not reach the rule sources.
func TestImportGeminiRules_ImportAllSkipsLinksOutsideTheProject(t *testing.T) {
	root := hierarchicalGeminiWithOutsideLink(t)
	importAllSkippedEntryFiles = map[string]bool{}
	t.Cleanup(func() { importAllSkippedEntryFiles = nil })
	silence(t)

	dst := t.TempDir()
	n, err := importGeminiRules(root, dst, config.Sources{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("imported %d rules, want 2 (root and the in-project link)", n)
	}
	if rulesHolding(t, dst, "token=abc") {
		t.Error("the outside link must be skipped under import all")
	}
}

func TestImportGeminiRules_NamedImportFollowsLinksOutsideTheProject(t *testing.T) {
	root := hierarchicalGeminiWithOutsideLink(t)
	silence(t)

	dst := t.TempDir()
	if _, err := importGeminiRules(root, dst, config.Sources{}); err != nil {
		t.Fatal(err)
	}
	if !rulesHolding(t, dst, "token=abc") {
		t.Error("a named import follows the outside link")
	}
}
