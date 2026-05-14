package emit

import (
	"strings"
	"testing"
)

func TestMarshalJSONIndent_PreservesShellSymbols(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"command": "echo hi && echo bye | tee >out.log <in.txt",
	}
	raw, err := MarshalJSONIndent(in)
	if err != nil {
		t.Fatalf("MarshalJSONIndent: %v", err)
	}
	got := string(raw)
	for _, want := range []string{"&&", "|", ">", "<"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing literal %q: %s", want, got)
		}
	}
	for _, esc := range []string{"\\u0026", "\\u003c", "\\u003e"} {
		if strings.Contains(got, esc) {
			t.Errorf("output contains JSON unicode escape %q: %s", esc, got)
		}
	}
}

func TestMarshalJSONIndent_Indents(t *testing.T) {
	t.Parallel()
	raw, err := MarshalJSONIndent(map[string]any{"a": map[string]any{"b": 1}})
	if err != nil {
		t.Fatalf("MarshalJSONIndent: %v", err)
	}
	if !strings.Contains(string(raw), "\n    \"b\"") {
		t.Errorf("expected 2-space nested indent, got:\n%s", raw)
	}
	if strings.HasSuffix(string(raw), "\n") {
		t.Errorf("output should not end with trailing newline: %q", raw)
	}
}
