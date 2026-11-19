package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// frontmatterStyledReference is FrontmatterStyled as it was before it
// encoded all values in one pass: one Node.Encode per key. The
// differential test holds the new function to its bytes.
func frontmatterStyledReference(meta map[string]any, keys []string, styles map[string]yaml.Style) string {
	if len(meta) == 0 {
		return ""
	}
	ordered := orderedMetaKeys(meta, keys)
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range ordered {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		valNode := &yaml.Node{}
		if err := valNode.Encode(meta[k]); err != nil {
			return ""
		}
		applySourceStyle(valNode, styles[k])
		preferDoubleQuotes(valNode)
		root.Content = append(root.Content, keyNode, valNode)
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&yamlWriter{b: &buf})
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		_ = enc.Close()
		return ""
	}
	if err := enc.Close(); err != nil {
		return ""
	}
	return "---\n" + buf.String() + "---\n"
}

func TestFrontmatterStyledMatchesReferenceOnRepoSources(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Tracked files only: ignored folders hold generated output and
	// personal notes that differ between machines.
	listed, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.md", "*.mdc", "*.yaml", "*.yml").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	inputs := 0
	for rel := range strings.SplitSeq(strings.TrimRight(string(listed), "\x00"), "\x00") {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if ext := filepath.Ext(rel); ext == ".yaml" || ext == ".yml" {
			var meta map[string]any
			if yaml.Unmarshal(data, &meta) != nil || len(meta) == 0 {
				continue
			}
			inputs++
			assertFrontmatterMatches(t, rel, meta, nil, nil)
			continue
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
		if err != nil || len(entry.Meta) == 0 {
			continue
		}
		inputs++
		assertFrontmatterMatches(t, rel, entry.Meta, entry.MetaKeys, entry.MetaStyles)
	}
	if inputs < 500 {
		t.Fatalf("compared %d inputs; the walk should reach the repo's specs and adapter testdata", inputs)
	}
}

func TestFrontmatterStyledMatchesReferenceOnEdgeValues(t *testing.T) {
	values := map[string]any{
		"nil":        nil,
		"empty":      "",
		"multiline":  "first line\nsecond line\n",
		"trailing":   "no newline at end\nsecond",
		"spaces":     "  leading and trailing  ",
		"true":       "true",
		"yes":        "yes",
		"on":         "on",
		"null-str":   "null",
		"tilde":      "~",
		"version":    "1.0",
		"octal":      "0755",
		"int-str":    "42",
		"star":       "*glob",
		"bracket":    "[not a list]",
		"brace":      "{not: a map}",
		"hash":       "# not a comment",
		"colon":      "key: value",
		"dash":       "- item",
		"angle":      "<ver>",
		"quote":      `say "hi"`,
		"single":     "it's",
		"backslash":  `C:\path\to`,
		"unicode":    "naïve café 日本",
		"tab":        "a\tb",
		"bool":       true,
		"int":        7,
		"float":      1.5,
		"list":       []any{"Read", "Grep", "*", "[x]", nil, 3},
		"empty-list": []any{},
		"empty-map":  map[string]any{},
		"nested": map[string]any{
			"b":    []any{map[string]any{"z": "1.0", "a": []any{"x", "y"}}},
			"a":    "*",
			"true": "yes",
		},
		"strings": []string{"one", "two words", "#three"},
		"long":    strings.Repeat("word ", 40),
	}
	keys := []string{"nested", "list", "multiline", "true"}
	styles := map[string]yaml.Style{
		"empty":     yaml.DoubleQuotedStyle,
		"angle":     yaml.DoubleQuotedStyle,
		"version":   yaml.SingleQuotedStyle,
		"multiline": yaml.LiteralStyle,
		"trailing":  yaml.FoldedStyle,
		"list":      yaml.DoubleQuotedStyle,
		"nested":    yaml.FlowStyle,
		"unicode":   yaml.DoubleQuotedStyle,
	}
	assertFrontmatterMatches(t, "edge values", values, keys, styles)
	for k, v := range values {
		assertFrontmatterMatches(t, "edge value "+k, map[string]any{k: v}, nil, styles)
	}
	assertFrontmatterMatches(t, "odd keys", map[string]any{
		"true": 1, "1.0": 2, "null": 3, "*star": 4, "[x]": 5, "#hash": 6, "a b": 7, "": 8, "multi\nline": 9, "\tfoo\nbar": 10,
	}, []string{"null", "absent", "true"}, nil)
	assertFrontmatterMatches(t, "invalid utf-8 key", map[string]any{"bad\xffkey": "v", "good": "w"}, nil, nil)
	assertFrontmatterMatches(t, "invalid utf-8 value", map[string]any{"k": "bad\xffvalue"}, nil, nil)
}

func assertFrontmatterMatches(t *testing.T, name string, meta map[string]any, keys []string, styles map[string]yaml.Style) {
	t.Helper()
	cases := []struct {
		keys   []string
		styles map[string]yaml.Style
	}{{keys, styles}, {keys, nil}, {nil, nil}}
	for _, c := range cases {
		want := frontmatterStyledReference(meta, c.keys, c.styles)
		if got := FrontmatterStyled(meta, c.keys, c.styles); got != want {
			t.Errorf("%s: frontmatter differs from the per-key reference\n got: %q\nwant: %q", name, got, want)
		}
	}
}
