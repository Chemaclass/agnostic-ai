package hookrun

import (
	"errors"
	"testing"
)

func TestAddsContext_TraeUsesTheEventOutputContract(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		result      Result
		want        bool
	}{
		{"startup text", "SessionStart", Result{Stdout: "context"}, true},
		{"prompt text", "UserPromptSubmit", Result{Stdout: "context"}, true},
		{"startup JSON", "SessionStart", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, true},
		{"prompt JSON", "UserPromptSubmit", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, true},
		{"pretool JSON", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, true},
		{"posttool JSON", "PostToolUse", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, true},
		{"notification JSON ignored", "Notification", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
		{"stop JSON ignored", "Stop", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
		{"unknown event", "Other", Result{Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
		{"tool text ignored", "PreToolUse", Result{Stdout: "context"}, false},
		{"empty text", "SessionStart", Result{Stdout: " \n"}, false},
		{"JSON without context", "SessionStart", Result{Stdout: `{"continue":true}`}, false},
		{"failed text", "SessionStart", Result{Exit: 1, Stdout: "context"}, false},
		{"failed JSON", "SessionStart", Result{Exit: 1, Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
		{"timeout JSON", "SessionStart", Result{TimedOut: true, Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
		{"start error JSON", "SessionStart", Result{StartErr: errors.New("cannot start"), Stdout: `{"hookSpecificOutput":{"additionalContext":"context"}}`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AddsContext("trae", tc.event, tc.result); got != tc.want {
				t.Errorf("AddsContext = %t, want %t", got, tc.want)
			}
		})
	}
}
