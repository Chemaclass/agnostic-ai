package gemini

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Gemini has no args field and runs the command through bash, or
// PowerShell on Windows. A bare `node` would read the event JSON on
// stdin as its program.
func TestEmit_ExecFormHookFoldsQuotedArgs(t *testing.T) {
	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "BeforeTool", "command": "node", "args": []any{"guard.js", "--strict"}}})
	got := readTargetFile(t, ".gemini/settings.json")
	if !strings.Contains(got, `"command": "node 'guard.js' '--strict'"`) || strings.Contains(got, `"args"`) {
		t.Errorf("settings.json:\n%s", got)
	}
}
