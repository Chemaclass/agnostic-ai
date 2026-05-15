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
	if string(got) != "hello\n" {
		t.Errorf("expected 'hello\\n' (normalized trailing newline), got %q", got)
	}
}

func TestWriteFile_NormalizesTrailingNewlines(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ name, in, want string }{
		{"no-newline", "hello", "hello\n"},
		{"single-newline", "hello\n", "hello\n"},
		{"double-newline", "hello\n\n", "hello\n"},
		{"many-newlines", "hello\n\n\n\n", "hello\n"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".txt")
			if err := WriteFile(path, tc.in, false); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 0 {
					t.Errorf("empty in -> want empty file, got %q", got)
				}
				return
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("in=%q want=%q got=%q", tc.in, tc.want, got)
			}
		})
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

func TestStartCounting_CountsRealWrites(t *testing.T) {
	dir := t.TempDir()
	StartCounting()
	if err := WriteFile(filepath.Join(dir, "a.txt"), "aaa", false); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(dir, "b.txt"), "bbb", false); err != nil {
		t.Fatal(err)
	}
	n := StopCounting()
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}

func TestStartCounting_IgnoresDryRun(t *testing.T) {
	dir := t.TempDir()
	StartCounting()
	_ = WriteFile(filepath.Join(dir, "x.txt"), "xxx", true)
	n := StopCounting()
	if n != 0 {
		t.Fatalf("want 0 for dry-run, got %d", n)
	}
}

func TestStartCounting_IgnoresCaptureMode(t *testing.T) {
	StartCapture()
	StartCounting()
	_ = WriteFile("/nonexistent/fake.txt", "zzz", false)
	_ = StopCapture()
	n := StopCounting()
	if n != 0 {
		t.Fatalf("want 0 in capture mode, got %d", n)
	}
}

func TestStopCounting_WithoutStart_ReturnsZero(t *testing.T) {
	if n := StopCounting(); n != 0 {
		t.Fatalf("want 0 when not counting, got %d", n)
	}
}
