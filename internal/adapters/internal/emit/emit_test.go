package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFile_CreatesNestedDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.txt")
	if err := WriteFile(path, "hello", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestWriteFile_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "should-not-exist.txt")
	if err := WriteFile(path, "hello", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("dry-run wrote a file")
	}
}

func TestFrontmatter_Empty(t *testing.T) {
	if got := Frontmatter(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := Frontmatter(map[string]any{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFrontmatter_WithFields(t *testing.T) {
	got := Frontmatter(map[string]any{"name": "foo"})
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("expected leading ---, got %q", got)
	}
	if !strings.Contains(got, "name: foo") {
		t.Errorf("expected 'name: foo' in %q", got)
	}
	if !strings.HasSuffix(got, "---\n") {
		t.Errorf("expected trailing ---, got %q", got)
	}
}
