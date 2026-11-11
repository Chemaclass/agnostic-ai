package qwen

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitQwenHooks(t *testing.T, hooks ...spec.Entry) []map[string]any {
	t.Helper()
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(hooks), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(".qwen/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, group := range doc["hooks"].(map[string]any)["SessionStart"].([]any) {
		for _, handler := range group.(map[string]any)["hooks"].([]any) {
			out = append(out, handler.(map[string]any))
		}
	}
	return out
}

func sessionStartHook(name string, extra map[string]any) spec.Entry {
	meta := map[string]any{"event": "SessionStart"}
	for k, v := range extra {
		meta[k] = v
	}
	return spec.Entry{Kind: spec.KindHook, Name: name, Meta: meta}
}

// Qwen reads a command hook timeout of 1000 or more as milliseconds, so the
// portable seconds must be sent as milliseconds from 1000 up.
func TestEmit_CommandHookTimeoutUnit(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    float64
	}{
		{1, 1},
		{7, 7},
		{999, 999},
		{1000, 1000000},
		{1200, 1200000},
	} {
		handlers := emitQwenHooks(t, sessionStartHook("long", map[string]any{"command": "sleep 2", "timeout": tc.seconds}))
		if got := handlers[0]["timeout"]; got != tc.want {
			t.Errorf("timeout %d: native = %v, want %v", tc.seconds, got, tc.want)
		}
	}
}

func TestEmit_CommandHookTimeoutNativeOverrideIsKeptAsWritten(t *testing.T) {
	handlers := emitQwenHooks(t, sessionStartHook("native", map[string]any{
		"command": "sleep 2", "timeout": 1200, "x-qwen": map[string]any{"timeout": 1500000},
	}))
	if got := handlers[0]["timeout"]; got != float64(1500000) {
		t.Errorf("x-qwen.timeout = %v, want 1500000", got)
	}
}

func TestEmit_NonCommandHookTimeoutsStayInSeconds(t *testing.T) {
	handlers := emitQwenHooks(t,
		sessionStartHook("web", map[string]any{"type": "http", "url": "https://example.test/hook", "timeout": 1200}),
		sessionStartHook("ask", map[string]any{"type": "prompt", "prompt": "Check $ARGUMENTS", "timeout": 1200}),
	)
	for _, handler := range handlers {
		if handler["timeout"] != float64(1200) {
			t.Errorf("%v handler timeout = %v, want 1200 seconds", handler["type"], handler["timeout"])
		}
	}
}

func TestEmit_NativeHandlerGroupsKeepTheirTimeouts(t *testing.T) {
	handlers := emitQwenHooks(t, sessionStartHook("raw", map[string]any{
		"x-qwen": map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "native-guard", "timeout": 1200000},
			map[string]any{"type": "command", "command": "short-guard", "timeout": 20},
		}},
	}))
	if len(handlers) != 2 || handlers[0]["timeout"] != float64(1200000) || handlers[1]["timeout"] != float64(20) {
		t.Errorf("native handler timeouts changed: %#v", handlers)
	}
}
