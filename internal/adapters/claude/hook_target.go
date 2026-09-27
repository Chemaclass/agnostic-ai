package claude

import (
	"encoding/json"
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// setHookTargetEnv names the target in the settings `env`, which Claude
// Code passes to every hook it starts. A command hook has no env field
// of its own, and a command prefix would miss exec form and PowerShell.
// Tools that also run these hooks, such as Cursor and Copilot, do not
// read this env, so they are not mislabeled as claude. A value set by
// hand stays; the one sync wrote goes with the last command hook.
func setHookTargetEnv(doc *emit.OrderedJSON, want bool) error {
	env := map[string]any{}
	if raw, ok := doc.Get("env"); ok {
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("claude settings: parse env: %w", err)
		}
	}
	next := emit.WithoutHookTarget(env, any(target))
	if want {
		next = emit.WithHookTarget(env, any(target))
	}
	if len(next) == len(env) {
		return nil
	}
	if len(next) == 0 {
		doc.Delete("env")
		return nil
	}
	if err := doc.Set("env", next); err != nil {
		return fmt.Errorf("claude settings: marshal env: %w", err)
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
