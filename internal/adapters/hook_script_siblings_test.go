package adapters

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Sync joins hooks into Cline's event script by the event their
// `x-cline` override names, so the siblings follow it too.
func TestHookScriptSiblings_ReadTheOverriddenEvent(t *testing.T) {
	hook := func(name string) spec.Entry {
		return spec.Entry{Kind: spec.KindHook, Name: name, Path: name + ".yaml", Meta: map[string]any{
			"event": "PostToolUse", "command": "echo " + name,
			"x-cline": map[string]any{"event": "PreToolUse"},
		}}
	}
	a, b := hook("a"), hook("b")
	got := HookScriptSiblings(&config.Config{}, "cline", []spec.Entry{a, b}, TargetHook("cline", a))
	if !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("siblings = %v, want [b]", got)
	}
}
