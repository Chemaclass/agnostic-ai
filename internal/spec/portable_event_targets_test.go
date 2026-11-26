package spec

import (
	"reflect"
	"testing"
)

func TestPortableEventTargets_ListsTargetsWithTheEvent(t *testing.T) {
	want := []string{"augment", "claude", "codebuddy", "codex", "copilot", "cursor", "factory", "gemini", "goose", "openhands", "qoder", "qwen"}
	if got := PortableEventTargets("session-start"); !reflect.DeepEqual(got, want) {
		t.Errorf("session-start targets = %v, want %v", got, want)
	}
	if got := PortableEventTargets("no-such-event"); got != nil {
		t.Errorf("unknown event targets = %v, want none", got)
	}
}
