package emit

import (
	"encoding/json"
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

func TestOrderedJSON_PreservesTopLevelKeyOrder(t *testing.T) {
	t.Parallel()
	src := []byte(`{
  "hooks": {"PreToolUse": []},
  "statusLine": {"type": "command", "command": "x"},
  "enabledPlugins": {"a": true}
}`)
	doc := NewOrderedJSON()
	if err := json.Unmarshal(src, doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := doc.Keys(); !equalSlice(got, []string{"hooks", "statusLine", "enabledPlugins"}) {
		t.Fatalf("keys not preserved: %v", got)
	}
	out, err := MarshalJSONIndent(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if i := strings.Index(s, `"statusLine"`); i < 0 || i < strings.Index(s, `"hooks"`) {
		t.Errorf("statusLine must follow hooks in output:\n%s", s)
	}
	// statusLine inner keys must NOT be alpha-sorted (type before command).
	idxType := strings.Index(s, `"type"`)
	idxCmd := strings.Index(s, `"command"`)
	if !(idxType >= 0 && idxCmd > idxType) {
		t.Errorf("nested key order lost (type should precede command):\n%s", s)
	}
}

func TestOrderedJSON_SetAppendsNewKeyAtEnd(t *testing.T) {
	t.Parallel()
	src := []byte(`{"alpha": 1, "zeta": 2}`)
	doc := NewOrderedJSON()
	if err := json.Unmarshal(src, doc); err != nil {
		t.Fatal(err)
	}
	if err := doc.Set("middle", "added"); err != nil {
		t.Fatal(err)
	}
	if got := doc.Keys(); !equalSlice(got, []string{"alpha", "zeta", "middle"}) {
		t.Errorf("expected new key at tail, got %v", got)
	}
}

func TestOrderedJSON_SetExistingKeyKeepsPosition(t *testing.T) {
	t.Parallel()
	src := []byte(`{"alpha": 1, "beta": 2, "gamma": 3}`)
	doc := NewOrderedJSON()
	if err := json.Unmarshal(src, doc); err != nil {
		t.Fatal(err)
	}
	if err := doc.Set("beta", "replaced"); err != nil {
		t.Fatal(err)
	}
	if got := doc.Keys(); !equalSlice(got, []string{"alpha", "beta", "gamma"}) {
		t.Errorf("position changed on overwrite: %v", got)
	}
	out, _ := MarshalJSONIndent(doc)
	if !strings.Contains(string(out), `"beta": "replaced"`) {
		t.Errorf("expected replaced value, got:\n%s", out)
	}
}

func TestOrderedJSON_EmptyMarshalsAsBrackets(t *testing.T) {
	t.Parallel()
	doc := NewOrderedJSON()
	out, err := MarshalJSONIndent(doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "{}" {
		t.Errorf("want %q, got %q", "{}", out)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
