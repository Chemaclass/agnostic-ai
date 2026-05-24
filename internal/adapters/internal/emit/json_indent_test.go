package emit

import (
	"strings"
	"testing"
)

func TestDetectJSONIndent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"2-space", "{\n  \"a\": 1\n}", "  "},
		{"4-space", "{\n    \"a\": 1\n}", "    "},
		{"tab", "{\n\t\"a\": 1\n}", "\t"},
		{"single line", `{"a": 1}`, ""},
		{"empty", "", ""},
		{"blank line before first nested", "{\n\n  \"a\": 1\n}", "  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetectJSONIndent([]byte(c.in)); got != c.want {
				t.Errorf("DetectJSONIndent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMarshalJSONIndentWith_PreservesIndent(t *testing.T) {
	doc := NewOrderedJSON()
	_ = doc.Set("a", 1)

	raw, err := MarshalJSONIndentWith(doc, "    ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `    "a": 1`) {
		t.Errorf("expected 4-space indent in:\n%s", raw)
	}

	raw, err = MarshalJSONIndentWith(doc, "\t")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\t\"a\": 1") {
		t.Errorf("expected tab indent in:\n%s", raw)
	}

	// Empty indent falls back to default 2-space.
	raw, err = MarshalJSONIndentWith(doc, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `  "a": 1`) {
		t.Errorf("expected 2-space fallback in:\n%s", raw)
	}
}
