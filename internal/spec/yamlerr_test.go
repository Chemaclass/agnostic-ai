package spec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormatYAMLError_LineAndCol(t *testing.T) {
	err := errors.New("yaml: line 3: column 5: did not find expected key")
	got := formatYAMLError("rules/x.md", err, 1).Error()
	want := "rules/x.md:4:5: did not find expected key"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatYAMLError_LineOnly(t *testing.T) {
	err := errors.New("yaml: line 2: mapping values are not allowed in this context")
	got := formatYAMLError("hooks/h.yaml", err, 0).Error()
	want := "hooks/h.yaml:2:1: mapping values are not allowed in this context"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatYAMLError_Unparseable(t *testing.T) {
	err := errors.New("totally unrecognized format")
	got := formatYAMLError("p.md", err, 0).Error()
	want := "p.md:1:1: totally unrecognized format"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatYAMLError_NilReturnsNil(t *testing.T) {
	if formatYAMLError("x", nil, 0) != nil {
		t.Error("expected nil error to round-trip")
	}
}

func TestFormatYAMLError_TypeErrorUsesFirstMessage(t *testing.T) {
	te := &yaml.TypeError{Errors: []string{
		"yaml: line 4: column 1: cannot unmarshal !!str into int",
	}}
	got := formatYAMLError("mcps/m.yaml", te, 0).Error()
	want := "mcps/m.yaml:4:1: cannot unmarshal !!str into int"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseMarkdown_FrontmatterErrorIncludesPathAndLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	// `:` at start of yaml line is invalid; yaml reports the offending
	// line within the frontmatter block, which the formatter then
	// shifts by +1 to account for the `---` opener.
	content := "---\n: : :\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := parseMarkdown(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	msg := err.Error()
	if !strings.Contains(msg, path) {
		t.Errorf("error %q missing path", msg)
	}
	if !strings.Contains(msg, ":2:") && !strings.Contains(msg, ":3:") {
		t.Errorf("error %q missing line number", msg)
	}
}

func TestParseYAML_ErrorIncludesPathAndPosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.yaml")
	// yaml.v3 reports a position for this construct; the exact line
	// number depends on the parser, so the test asserts the
	// `path:line:col:` envelope is present rather than a specific
	// line.
	content := "name: foo\nfoo: [1, 2,\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseYAML(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.HasPrefix(err.Error(), path+":") {
		t.Errorf("error %q does not start with %q", err.Error(), path+":")
	}
	parts := strings.SplitN(err.Error(), ":", 4)
	if len(parts) < 4 {
		t.Errorf("error %q missing line:col envelope", err.Error())
	}
}
