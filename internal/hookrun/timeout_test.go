package hookrun

import (
	"testing"
	"time"
)

// code.claude.com/docs/en/hooks: "Defaults: 600 for command ... Claude
// Code lowers the command ... default to 30 on UserPromptSubmit,
// PreModelSwitch, and PostModelSwitch, and to 10 on MessageDisplay."
func TestDefaultTimeout_FollowsEachTargetsEvent(t *testing.T) {
	for _, tc := range []struct {
		target, event string
		want          time.Duration
	}{
		{"claude", "PreToolUse", 600 * time.Second},
		{"claude", "UserPromptSubmit", 30 * time.Second},
		{"claude", "PostModelSwitch", 30 * time.Second},
		{"claude", "MessageDisplay", 10 * time.Second},
		{"codex", "UserPromptSubmit", 600 * time.Second},
		{"gemini", "BeforeTool", 60 * time.Second},
	} {
		if got := DefaultTimeout(tc.target, tc.event); got != tc.want {
			t.Errorf("DefaultTimeout(%s, %s) = %s, want %s", tc.target, tc.event, got, tc.want)
		}
	}
}
