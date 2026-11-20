package hookrun

import (
	"encoding/json"
	"testing"
)

func TestPermissionAllowReply(t *testing.T) {
	want := `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`
	for _, target := range []string{"claude", "codex"} {
		reply, ok := PermissionAllowReply(target)
		if !ok || string(reply) != want || !json.Valid(reply) {
			t.Errorf("%s reply = %s, %v; want %s", target, reply, ok, want)
		}
	}
	if reply, ok := PermissionAllowReply("kiro"); !ok || string(reply) != `{"hookSpecificOutput":{"decision":{"behavior":"allow"}}}` {
		t.Errorf("kiro reply = %s, %v; want no hookEventName", reply, ok)
	}
	if _, ok := PermissionAllowReply("copilot"); ok {
		t.Error("copilot has an allow reply; its hook skips deny rules")
	}
}
