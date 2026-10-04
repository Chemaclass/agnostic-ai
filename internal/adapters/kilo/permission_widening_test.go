package kilo

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Claude Code's Edit rules cover every tool that edits files, and Kilo
// keeps `write` apart from `edit`. An Edit deny or ask must also land on
// `write`, or a bare Write allow leaves the file writable. An Edit allow
// stays on `edit`, so it grants no more than it says.
func TestEmit_EditDenyAlsoBlocksWrite(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Write", "Edit(docs/*)"},
			"ask":   []any{"Edit(go.mod)"},
			"deny":  []any{"Edit(.env)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	want := map[string]map[string]any{
		"write": {"*": "allow", "go.mod": "ask", ".env": "deny"},
		"edit":  {"docs/*": "allow", "go.mod": "ask", ".env": "deny"},
	}
	for tool, patterns := range want {
		if !reflect.DeepEqual(permission[tool], toAnyMap(patterns)) {
			t.Errorf("permission[%q] = %#v, want %#v", tool, permission[tool], patterns)
		}
	}
}

// Kilo takes the last matching pattern, while Claude Code lets a deny or
// ask win over any allow. A narrower allow written after a broad deny
// would bypass it, so each tool's patterns go allow, ask, then deny.
func TestEmit_DenyAndAskComeAfterOverlappingAllows(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Write(secrets/token.txt)", "Bash(git push origin)", "Bash"},
			"ask":   []any{"Bash(git:*)"},
			"deny":  []any{"Edit(secrets/**)", "Bash(git push:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for tool, order := range map[string][]string{
		"write": {`"secrets/token.txt": "allow"`, `"secrets/**": "deny"`},
		"bash":  {`"*": "allow"`, `"git push origin": "allow"`, `"git *": "ask"`, `"git push *": "deny"`},
	} {
		data := raw[strings.Index(raw, `"`+tool+`": {`):]
		at := -1
		for _, want := range order {
			i := strings.Index(data, want)
			if i < 0 || i < at {
				t.Errorf("%s is missing or out of order in:\n%s", want, data)
			}
			at = i
		}
	}
}
