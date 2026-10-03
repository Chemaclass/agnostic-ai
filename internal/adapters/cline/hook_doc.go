package cline

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookScriptPath is the script sync writes the hooks of event to, or ""
// when event is not one of the file names Cline looks for.
func HookScriptPath(cfg *config.Config, event string) string {
	canonical, ok := clineHookEventByKey[strings.ToLower(strings.TrimSpace(event))]
	if !ok {
		return ""
	}
	return filepath.Join(emit.OutputHooksDir(cfg, target, defaultHooksDir), canonical)
}

// HookScript renders the event script sync writes for h alone, without
// the provenance comment, or "" when h has no command.
func HookScript(h spec.Entry) string {
	cmds := emit.HookCommands(h.Meta["command"])
	if len(cmds) == 0 {
		return ""
	}
	for i, cmd := range cmds {
		cmds[i] = emit.RewriteHookPath(cmd, target, h.Meta)
	}
	return hookScript(cmds)
}
