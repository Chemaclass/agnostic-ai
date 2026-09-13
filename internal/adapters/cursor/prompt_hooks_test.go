package cursor

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_PromptHooksKeepNativeFieldsAndCommandShape(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "prompt", Meta: map[string]any{"event": "beforeShellExecution", "type": "prompt", "prompt": "Allow read-only commands.", "model": "example-model", "timeout": 10, "loop_limit": nil, "failClosed": true, "matcher": "curl|wget"}},
		{Kind: spec.KindHook, Name: "command", Meta: map[string]any{"event": "beforeShellExecution", "command": "echo checked"}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".cursor/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"type": "prompt", "prompt": "Allow read-only commands.", "model": "example-model", "timeout": float64(10), "loop_limit": nil, "failClosed": true, "matcher": "curl|wget"},
		{"command": "echo checked"},
	}
	if !reflect.DeepEqual(doc.Hooks["beforeShellExecution"], want) {
		t.Errorf("hooks = %#v, want %#v", doc.Hooks, want)
	}
}
