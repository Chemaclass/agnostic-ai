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

func TestFrontmatterOrdered_PreservesSourceKeyOrder(t *testing.T) {
	meta := map[string]any{
		"name":          "changelog-keeper",
		"model":         "haiku",
		"description":   "Keeps changelog tidy",
		"allowed_tools": []string{"Read", "Edit"},
	}
	keys := []string{"name", "model", "description", "allowed_tools"}
	got := FrontmatterOrdered(meta, keys)
	// name must appear before description and allowed_tools.
	idxName := strings.Index(got, "name:")
	idxDesc := strings.Index(got, "description:")
	idxTools := strings.Index(got, "allowed_tools:")
	if idxName < 0 || idxDesc < 0 || idxTools < 0 {
		t.Fatalf("missing keys in output: %q", got)
	}
	if idxName >= idxDesc || idxDesc >= idxTools {
		t.Errorf("expected name<description<allowed_tools order, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_TwoSpaceArrayIndent(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"allowed_tools": []string{"Read", "Edit"}},
		[]string{"allowed_tools"},
	)
	// yaml.v3 default is 4 spaces; the helper forces 2.
	if !strings.Contains(got, "\n  - Read\n  - Edit\n") {
		t.Errorf("expected 2-space sequence indent, got:\n%s", got)
	}
	if strings.Contains(got, "    - Read") {
		t.Errorf("expected no 4-space indent, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_PromotesSingleToDoubleQuotes(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"argument-hint": "[findings-file-or-scope]"},
		[]string{"argument-hint"},
	)
	if !strings.Contains(got, `"[findings-file-or-scope]"`) {
		t.Errorf("expected double-quoted scalar, got:\n%s", got)
	}
	if strings.Contains(got, `'[findings-file-or-scope]'`) {
		t.Errorf("expected no single-quoted scalar, got:\n%s", got)
	}
}

// TestFrontmatterOrdered_QuotesScalarsWithAngleBrackets regresses
// DEFECT 5 of #218. yaml.v3 would otherwise emit a hand-quoted
// "<feature-or-problem-statement>" as a bare plain scalar, breaking
// byte-stable round-trip with source files that wrap such values in
// quotes for readability.
func TestFrontmatterOrdered_QuotesScalarsWithAngleBrackets(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"argument-hint": "<feature-or-problem-statement>"},
		[]string{"argument-hint"},
	)
	if !strings.Contains(got, `"<feature-or-problem-statement>"`) {
		t.Errorf("expected angle-bracket scalar wrapped in double quotes, got:\n%s", got)
	}
}

// TestFrontmatterOrdered_PreservesPlainScalar regresses #226. A short
// hand-authored plain scalar must round-trip unchanged through the
// emitter (no double quotes, no folding).
func TestFrontmatterOrdered_PreservesPlainScalar(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"description": "Review code changes for quality."},
		[]string{"description"},
	)
	if !strings.Contains(got, "description: Review code changes for quality.\n") {
		t.Errorf("expected plain scalar preserved, got:\n%s", got)
	}
	if strings.Contains(got, `description: "`) {
		t.Errorf("plain scalar was force-quoted, got:\n%s", got)
	}
}

// TestFrontmatterOrdered_PreservesLongPlainDescriptions regresses #226.
// Long plain scalars stay plain on round-trip: yaml.v3 v3.0.1 does not
// auto-wrap plain scalars, so promoting them to double-quoted would
// only add noise to byte-stable diffs after `import -> sync`.
func TestFrontmatterOrdered_PreservesLongPlainDescriptions(t *testing.T) {
	long := "Execute the implementation planning workflow using the plan template to generate design artifacts."
	got := FrontmatterOrdered(
		map[string]any{"description": long},
		[]string{"description"},
	)
	if strings.Contains(got, `"`+long+`"`) {
		t.Errorf("long plain description was force-quoted, want plain:\n%s", got)
	}
	if !strings.Contains(got, "description: "+long+"\n") {
		t.Errorf("expected plain `description: <long>` on one line, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_UnhintedKeysAppendAlphabetically(t *testing.T) {
	meta := map[string]any{"zeta": 1, "alpha": 2, "name": "foo"}
	got := FrontmatterOrdered(meta, []string{"name"})
	idxName := strings.Index(got, "name:")
	idxAlpha := strings.Index(got, "alpha:")
	idxZeta := strings.Index(got, "zeta:")
	if idxName >= idxAlpha || idxAlpha >= idxZeta {
		t.Errorf("expected hinted-then-alpha order, got:\n%s", got)
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
