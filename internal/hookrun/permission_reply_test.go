package hookrun

import (
	"encoding/json"
	"testing"
)

func TestPermissionAllowReply(t *testing.T) {
	want := `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`
	for _, target := range []string{"claude", "codex", "qoder"} {
		reply, ok := PermissionAllowReply(target)
		if !ok || string(reply) != want || !json.Valid(reply) {
			t.Errorf("%s reply = %s, %v; want %s", target, reply, ok, want)
		}
	}
	for _, target := range []string{"kiro", "qwen"} {
		if reply, ok := PermissionAllowReply(target); !ok || string(reply) != `{"hookSpecificOutput":{"decision":{"behavior":"allow"}}}` {
			t.Errorf("%s reply = %s, %v; want no hookEventName", target, reply, ok)
		}
	}
	if _, ok := PermissionAllowReply("copilot"); ok {
		t.Error("copilot has an allow reply; its hook skips deny rules")
	}
}

func TestPermissionDenyReply(t *testing.T) {
	want := `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"Denied in the agnostic-ai companion"}}}`
	for _, target := range []string{"claude", "codex"} {
		reply, ok := PermissionDenyReply(target)
		if !ok || string(reply) != want || !json.Valid(reply) {
			t.Errorf("%s reply = %s, %v; want %s", target, reply, ok, want)
		}
	}
	// Codex refuses a reply with an unknown field, so the decision holds
	// only behavior and message.
	var parsed struct {
		HookSpecificOutput struct {
			Decision map[string]any `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	reply, _ := PermissionDenyReply("codex")
	if err := json.Unmarshal(reply, &parsed); err != nil || len(parsed.HookSpecificOutput.Decision) != 2 {
		t.Errorf("codex decision = %v, %v", parsed.HookSpecificOutput.Decision, err)
	}
	for _, target := range []string{"kiro", "qwen", "qoder", "copilot"} {
		if _, ok := PermissionDenyReply(target); ok {
			t.Errorf("%s has a deny reply; its deny was not measured", target)
		}
	}
}
