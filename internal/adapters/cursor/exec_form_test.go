package cursor

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Cursor's hook definition has only `command`, run through `$SHELL -c`,
// or PowerShell on Windows. A bare `node` would read the event JSON on
// stdin as its program.
func TestEmit_ExecFormHookFoldsQuotedArgs(t *testing.T) {
	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "beforeShellExecution", "command": "node", "args": []any{"guard.js"}}})
	got := readTargetFile(t, ".cursor/hooks.json")
	if !strings.Contains(got, `"command": "node 'guard.js'"`) || strings.Contains(got, `"args"`) {
		t.Errorf("hooks.json:\n%s", got)
	}
}
