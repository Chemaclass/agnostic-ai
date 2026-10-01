package emit

import "sync"

// modelAliases maps a target to the model aliases agnostic-ai resolves
// for it, for vendors that document only exact ids. Each release pins
// the ids, so a project that pins `requires` writes the same ones on
// every machine. Claude Code resolves its own aliases and has no entry.
var modelAliases = map[string]map[string]string{
	// learn.chatgpt.com/docs/models
	"codex": {
		"sol":   "gpt-6.1-sol",
		"luna":  "gpt-6-luna",
		"astra": "gpt-6-astra",
	},
}

// ModelAlias returns the id alias names for target, and whether it is one.
func ModelAlias(target, alias string) (string, bool) {
	model, ok := modelAliases[target][alias]
	return model, ok
}

// ModelAliasIn returns the alias the model target gets from meta
// resolves through, or "" when it gets an exact id. `x-<target>.model`
// is literal, so it never names an alias.
func ModelAliasIn(meta map[string]any, target string) string {
	if literalTargetModel(meta, target) {
		return ""
	}
	picked := map[string]any{"model": meta["model"]}
	keys := []string{"model"}
	collapseTargetMap(picked, &keys, "model", target)
	alias, _ := picked["model"].(string)
	if _, ok := ModelAlias(target, alias); !ok {
		return ""
	}
	return alias
}

// resolvedAliases records each alias a render resolved, by target, so a
// sync can tell when an upgrade moved one.
var resolvedAliases = struct {
	mu       sync.Mutex
	byTarget map[string]map[string]string
}{}

// resolveModelAlias swaps an alias under out["model"] for target's id.
func resolveModelAlias(out map[string]any, target string) {
	if alias, ok := out["model"].(string); ok {
		if model, ok := ModelAlias(target, alias); ok {
			out["model"] = model
			recordResolvedAlias(target, alias, model)
		}
	}
}

func recordResolvedAlias(target, alias, model string) {
	resolvedAliases.mu.Lock()
	defer resolvedAliases.mu.Unlock()
	if resolvedAliases.byTarget == nil {
		resolvedAliases.byTarget = map[string]map[string]string{}
	}
	if resolvedAliases.byTarget[target] == nil {
		resolvedAliases.byTarget[target] = map[string]string{}
	}
	resolvedAliases.byTarget[target][alias] = model
}

// TakeResolvedAliases returns the aliases resolved since the last call,
// by target, and clears the record.
func TakeResolvedAliases() map[string]map[string]string {
	resolvedAliases.mu.Lock()
	defer resolvedAliases.mu.Unlock()
	taken := resolvedAliases.byTarget
	resolvedAliases.byTarget = nil
	return taken
}

func literalTargetModel(meta map[string]any, target string) bool {
	custom, _ := meta[XPrefix+target].(map[string]any)
	_, set := custom["model"]
	return set
}
