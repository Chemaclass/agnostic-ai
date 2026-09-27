package claude

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// dropStaleHookTargetEnv removes the value an earlier sync wrote once no
// command hook is left. It runs before the spec and config layers, so a
// value pinned there survives.
func dropStaleHookTargetEnv(doc *emit.OrderedJSON, hooks []spec.Entry) error {
	if hasCommandHook(hooks) {
		return nil
	}
	return setHookTargetEnv(doc, false)
}

// addHookTargetEnv names the target in the settings `env`, which Claude
// Code passes to every hook it starts: a command hook has no env field,
// and a command prefix would miss exec form and PowerShell. Cursor and
// Copilot also run these hooks but do not read this env. It runs after
// the config layer and before `x-claude`, so either can set its own value.
func addHookTargetEnv(doc *emit.OrderedJSON, hooks []spec.Entry) error {
	if !hasCommandHook(hooks) {
		return nil
	}
	return setHookTargetEnv(doc, true)
}

func setHookTargetEnv(doc *emit.OrderedJSON, want bool) error {
	if err := emit.SetHookTargetEnv(doc, target, want); err != nil {
		return fmt.Errorf("claude settings: %w", err)
	}
	return nil
}

func hasCommandHook(hooks []spec.Entry) bool {
	for _, h := range hooks {
		kind, _ := h.Meta["type"].(string)
		event, _ := h.Meta["event"].(string)
		if (kind == "" || kind == "command") && event != "" && len(hookCommands(h.Meta["command"])) > 0 {
			return true
		}
	}
	return false
}
