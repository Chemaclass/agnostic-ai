package qwen

import (
	"os"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitAgents_WritesBackgroundAndHooks(t *testing.T) {
	testutil.TempCwd(t)
	hooks := map[string]any{"PreToolUse": []any{map[string]any{
		"matcher": "Bash",
		"hooks":   []any{map[string]any{"type": "command", "command": "./guard.sh"}},
	}}}
	err := New().EmitAgents(emit.NewSession(), []spec.Entry{{
		Kind: spec.KindAgent, Name: "rev",
		Meta: map[string]any{"description": "Reviewer", "background": true, "hooks": hooks},
		Body: "Review.",
	}}, ".qwen/agents", false)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(".qwen/agents/rev.md")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := spec.ParseMarkdownBytes(spec.KindAgent, raw)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if entry.Meta["background"] != true {
		t.Errorf("background missing:\n%s", raw)
	}
	if !reflect.DeepEqual(entry.Meta["hooks"], hooks) {
		t.Errorf("hooks = %#v, want %#v\n%s", entry.Meta["hooks"], hooks, raw)
	}
}
