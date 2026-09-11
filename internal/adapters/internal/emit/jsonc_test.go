package emit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"line comment", "{\n  // note\n  \"a\": 1\n}", `{"a":1}`},
		{"trailing comment on a value line", "{\n  \"a\": 1 // note\n}", `{"a":1}`},
		{"block comment", "{ /* note */ \"a\": 1 }", `{"a":1}`},
		{"multi-line block comment", "{\n/* one\n   two */\n  \"a\": 1\n}", `{"a":1}`},
		{"trailing comma in object", `{"a": 1,}`, `{"a":1}`},
		{"trailing comma in array", `{"a": [1, 2,]}`, `{"a":[1,2]}`},
		{"trailing comma before a newline", "{\n  \"a\": 1,\n}", `{"a":1}`},
		{"nested trailing commas", "{\n  \"a\": [1,],\n}", `{"a":[1]}`},
		{"empty object", `{}`, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped, _ := StripJSONC([]byte(tt.in))
			var got map[string]any
			if err := json.Unmarshal(stripped, &got); err != nil {
				t.Fatalf("stripped output still invalid: %v\n%s", err, stripped)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tt.want {
				t.Errorf("got %s, want %s", raw, tt.want)
			}
		})
	}
}

// A `//` inside a string literal is data, not a comment. A URL value is
// the case that hits this in practice: every remote MCP server spec
// carries one.
func TestStripJSONC_LeavesStringContentsAlone(t *testing.T) {
	const in = `{
  "url": "https://mcp.linear.app/mcp",
  "note": "/* not a comment */ and // not one either",
  "escaped": "a \" quote then // still inside the string",
  "comma": "trailing, "
}`
	stripped, hadComments := StripJSONC([]byte(in))
	if hadComments {
		t.Error("a comment marker inside a string is not a comment")
	}
	var got map[string]string
	if err := json.Unmarshal(stripped, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, stripped)
	}
	want := map[string]string{
		"url":     "https://mcp.linear.app/mcp",
		"note":    "/* not a comment */ and // not one either",
		"escaped": `a " quote then // still inside the string`,
		"comma":   "trailing, ",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}

// The worked example on kilo.ai/docs/customize/custom-rules, verbatim.
// It carries a `//` comment on its first line, a second one inside the
// array, and two trailing commas, so it is the exact shape
// `encoding/json` rejects.
func TestStripJSONC_ParsesKiloDocumentedExample(t *testing.T) {
	const in = `// kilo.jsonc
{
  "instructions": [
    ".kilo/rules/formatting.md",
    // ".kilo/rules/experimental.md" -- temporarily disabled
    ".kilo/rules/naming_conventions.md",
  ],
}`
	stripped, hadComments := StripJSONC([]byte(in))
	if !hadComments {
		t.Error("the vendor example carries two comments")
	}
	doc := NewOrderedJSON()
	if err := json.Unmarshal(stripped, doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, stripped)
	}
	raw, ok := doc.Get("instructions")
	if !ok {
		t.Fatal("instructions key missing")
	}
	for _, want := range []string{"formatting.md", "naming_conventions.md"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("instructions lost %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "experimental.md") {
		t.Errorf("commented-out entry should not be read back: %s", raw)
	}
}

// Byte offsets must not shift, or a json.SyntaxError points at the
// wrong column of the file the user has open.
func TestStripJSONC_PreservesLength(t *testing.T) {
	const in = "{\n  // note\n  \"a\": 1,\n  /* b */ \"c\": 2,\n}"
	stripped, _ := StripJSONC([]byte(in))
	if len(stripped) != len(in) {
		t.Errorf("length changed: got %d, want %d", len(stripped), len(in))
	}
}

func TestStripJSONC_DoesNotMutateInput(t *testing.T) {
	const in = "{ // note\n}"
	data := []byte(in)
	StripJSONC(data)
	if string(data) != in {
		t.Errorf("input mutated: %q", data)
	}
}

func TestStripJSONC_ReportsComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain json", `{"a": 1}`, false},
		{"url value", `{"url": "https://example.com"}`, false},
		{"trailing comma only", `{"a": 1,}`, false},
		{"line comment", "{\n// x\n}", true},
		{"block comment", `{/* x */}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := StripJSONC([]byte(tt.in)); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
