package qoder

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Qoder renders its own `.qoder/agents/<name>.md`, so the two fields it
// documents for a subagent's safety boundary belong in that file.
func TestAgentMarkdown_EmitsPermissionModeAndHooks(t *testing.T) {
	got := agentMarkdown(spec.Entry{
		Kind: spec.KindAgent, Name: "guard", Body: "Be careful.",
		Meta: map[string]any{
			"description":    "Careful agent",
			"permissionMode": "acceptEdits",
			"hooks": map[string]any{
				"PreToolUse": []any{map[string]any{"command": "./check.sh"}},
			},
		},
	})
	for _, want := range []string{"permissionMode: acceptEdits", "hooks:", "PreToolUse"} {
		if !strings.Contains(got, want) {
			t.Errorf("agent frontmatter missing %q:\n%s", want, got)
		}
	}
}

// A value outside Qoder's documented set parses and then inherits the
// parent session mode, which is a silently wider boundary than asked
// for. Claude's `manual` alias is the realistic way to hit it.
func TestPermissionModes_CoverQodersDocumentedSetOnly(t *testing.T) {
	for _, mode := range []string{"default", "acceptEdits", "bypassPermissions", "dontAsk", "auto", "plan"} {
		if !permissionModes[mode] {
			t.Errorf("%q is documented by Qoder but not accepted", mode)
		}
	}
	for _, mode := range []string{"manual", "", "yolo"} {
		if permissionModes[mode] {
			t.Errorf("%q is not a Qoder permission mode", mode)
		}
	}
}

// A subagent runs a narrower event set than the project hook file, so
// an event valid at project scope can be inert at agent scope.
func TestUnknownAgentHookEvents(t *testing.T) {
	hooks := map[string]any{
		"PreToolUse":       []any{},
		"SubagentStop":     []any{},
		"UserPromptSubmit": []any{},
		"SessionStart":     []any{},
	}
	got := unknownAgentHookEvents(hooks)
	want := []string{"SessionStart", "UserPromptSubmit"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("unknownAgentHookEvents = %v, want %v", got, want)
	}
	if got := unknownAgentHookEvents(nil); len(got) != 0 {
		t.Errorf("nil hooks returned %v", got)
	}
}
