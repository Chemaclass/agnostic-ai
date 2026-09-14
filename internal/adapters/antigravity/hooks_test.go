package antigravity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// emitHooksBundle syncs hooks only and returns the parsed
// .agents/hooks.json, plus its raw bytes for order-sensitive checks.
func emitHooksBundle(t *testing.T, entries []spec.Entry) (map[string]any, string) {
	t.Helper()
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	data, err := os.ReadFile(defaultHooksFile)
	if err != nil {
		t.Fatalf("read %s: %v", defaultHooksFile, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", defaultHooksFile, err, data)
	}
	return doc, string(data)
}

func TestEmitHooks_ToolEventNestsUnderMatcherGroup(t *testing.T) {
	doc, _ := emitHooksBundle(t, []spec.Entry{{
		Kind: spec.KindHook, Name: "my-linter-hook",
		Meta: map[string]any{
			"event":   "PostToolUse",
			"matcher": "run_command",
			"command": "./scripts/lint.sh",
			"timeout": 10,
		},
	}})

	def, ok := doc["my-linter-hook"].(map[string]any)
	if !ok {
		t.Fatalf("hook definition not keyed by spec name: %v", doc)
	}
	groups, ok := def["PostToolUse"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("PostToolUse is not a one-group array: %v", def)
	}
	group := groups[0].(map[string]any)
	if group["matcher"] != "run_command" {
		t.Errorf("matcher = %v, want run_command", group["matcher"])
	}
	handlers := group["hooks"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("hooks = %v, want one handler", handlers)
	}
	handler := handlers[0].(map[string]any)
	if handler["type"] != "command" || handler["command"] != "./scripts/lint.sh" {
		t.Errorf("handler = %v, want type command and the spec's command", handler)
	}
	if handler["timeout"] != float64(10) {
		t.Errorf("timeout = %v, want 10 seconds", handler["timeout"])
	}
}

// Antigravity documents a different array shape for the three non-tool
// events: "the structure is simpler (a list of handlers directly under
// the event key) and the matcher is ignored".
func TestEmitHooks_NonToolEventHoldsHandlersDirectly(t *testing.T) {
	doc, _ := emitHooksBundle(t, []spec.Entry{{
		Kind: spec.KindHook, Name: "reminder",
		Meta: map[string]any{"event": "PreInvocation", "command": "./scripts/reminder.sh"},
	}})

	def := doc["reminder"].(map[string]any)
	handlers, ok := def["PreInvocation"].([]any)
	if !ok || len(handlers) != 1 {
		t.Fatalf("PreInvocation is not a one-handler array: %v", def)
	}
	handler := handlers[0].(map[string]any)
	if handler["command"] != "./scripts/reminder.sh" {
		t.Errorf("command = %v, want the spec's command", handler["command"])
	}
	if _, nested := handler["hooks"]; nested {
		t.Errorf("handler is wrapped in a matcher group: %v", handler)
	}
	if _, matcher := handler["matcher"]; matcher {
		t.Errorf("handler carries a matcher Antigravity ignores: %v", handler)
	}
}

// `enabled` belongs to the hook definition, alongside the event keys,
// and is the inverse of the portable `disabled: true`.
func TestEmitHooks_DisabledSpecWritesEnabledFalse(t *testing.T) {
	doc, _ := emitHooksBundle(t, []spec.Entry{{
		Kind: spec.KindHook, Name: "safety-gate",
		Meta: map[string]any{
			"event":    "PreToolUse",
			"matcher":  "run_command",
			"command":  "./scripts/safety-check.sh",
			"disabled": true,
		},
	}})

	def := doc["safety-gate"].(map[string]any)
	if def["enabled"] != false {
		t.Errorf("enabled = %v, want false", def["enabled"])
	}
	if _, literal := def["disabled"]; literal {
		t.Errorf("wrote a literal disabled key Antigravity documents no slot for: %v", def)
	}
	group := def["PreToolUse"].([]any)[0].(map[string]any)
	if _, ok := group["hooks"]; !ok {
		t.Errorf("enabled displaced the event key: %v", def)
	}
}

func TestEmitHooks_EnabledIsAbsentWhenTheHookRuns(t *testing.T) {
	_, raw := emitHooksBundle(t, []spec.Entry{{
		Kind: spec.KindHook, Name: "linter",
		Meta: map[string]any{"event": "Stop", "command": "./scripts/lint.sh"},
	}})
	if strings.Contains(raw, "enabled") {
		t.Errorf("wrote enabled for a hook that is not disabled:\n%s", raw)
	}
}

// Every spec is its own top-level definition, so two hooks on one event
// stay separate rather than merging into a shared event array.
func TestEmitHooks_EachSpecIsItsOwnDefinitionInSpecOrder(t *testing.T) {
	doc, raw := emitHooksBundle(t, []spec.Entry{
		{Kind: spec.KindHook, Name: "first", Meta: map[string]any{"event": "PreToolUse", "command": "a.sh"}},
		{Kind: spec.KindHook, Name: "second", Meta: map[string]any{"event": "PreToolUse", "command": "b.sh"}},
	})
	if len(doc) != 2 {
		t.Fatalf("definitions = %v, want one per spec", doc)
	}
	if strings.Index(raw, `"first"`) > strings.Index(raw, `"second"`) {
		t.Errorf("definitions are not in spec order:\n%s", raw)
	}
}

// A list-valued command produces one handler per command inside the
// same definition, the same way every other hook adapter expands it.
func TestEmitHooks_CommandListExpandsToOneHandlerEach(t *testing.T) {
	doc, _ := emitHooksBundle(t, []spec.Entry{{
		Kind: spec.KindHook, Name: "pair",
		Meta: map[string]any{"event": "Stop", "command": []any{"a.sh", "b.sh"}},
	}})
	handlers := doc["pair"].(map[string]any)["Stop"].([]any)
	if len(handlers) != 2 {
		t.Fatalf("handlers = %v, want one per command", handlers)
	}
}

// An event outside Antigravity's documented five would land in the file
// with no handler behind it, so it is skipped instead.
func TestEmitHooks_UndocumentedEventWritesNothing(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{
		Kind: spec.KindHook, Name: "compaction",
		Meta: map[string]any{"event": "PostCompaction", "command": "a.sh"},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(defaultHooksFile); !os.IsNotExist(err) {
		t.Errorf("%s exists for an event Antigravity does not run", defaultHooksFile)
	}
}

func TestEmitHooks_NoHooksWritesNoFile(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(defaultHooksFile); !os.IsNotExist(err) {
		t.Errorf("%s exists with no hook specs", defaultHooksFile)
	}
}

func TestEmitHooks_HooksFileOverrideIsHonored(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"antigravity": {HooksFile: ".agents/custom-hooks.json"},
	}}
	entries := []spec.Entry{{
		Kind: spec.KindHook, Name: "h", Meta: map[string]any{"event": "Stop", "command": "a.sh"},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !pathSetContains(testutil.WalkRel(t, dir), ".agents/custom-hooks.json") {
		t.Errorf("override path not written (paths: %v)", testutil.WalkRel(t, dir))
	}
	if _, err := os.Stat(defaultHooksFile); !os.IsNotExist(err) {
		t.Errorf("default path written alongside the override")
	}
}
