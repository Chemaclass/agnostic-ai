package claude

import (
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// GuardCursorCopies returns hooks with each command of a portable hook
// that reaches Cursor too prefixed by a check that exits 0 under Cursor,
// when cfg syncs cursor. Cursor runs `.claude/settings.json` hooks beside
// its own (third-party hooks, on by default), so the hook would run
// twice there; with the check it runs once, as Cursor's own copy. Claude
// Code's settings env sets claude, and an unset variable runs the hook,
// so Claude Code never skips it. An exec-form or PowerShell handler has
// no POSIX shell to read the check, so it keeps running twice.
func GuardCursorCopies(cfg *config.Config, hooks []spec.Entry) []spec.Entry {
	if cfg == nil || !slices.Contains(cfg.Targets, "cursor") {
		return hooks
	}
	out := slices.Clone(hooks)
	for i, h := range out {
		kind, _ := h.Meta["type"].(string)
		shell, _ := h.Meta["shell"].(string)
		if !h.ReachesPortably("cursor") || kind != "" && kind != "command" || shell == "powershell" || len(emit.StringSlice(h.Meta["args"])) > 0 {
			continue
		}
		var guarded []any
		for _, command := range hookCommands(h.Meta["command"]) {
			guarded = append(guarded, emit.GuardCursorCopy(command))
		}
		if len(guarded) == 0 {
			continue
		}
		meta := maps.Clone(h.Meta)
		if len(guarded) == 1 {
			meta["command"] = guarded[0]
		} else {
			meta["command"] = guarded
		}
		out[i].Meta = meta
	}
	return out
}
