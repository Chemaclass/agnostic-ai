package windsurf

import (
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// memoryWriteRules allows writes to the repo store of personal memory
// when it lies outside the checkout. Devin asks before a write outside
// the workspace, even in Accept Edits mode
// (docs.devin.ai/cli/reference/permissions).
func memoryWriteRules(sess *emit.Session, cfg *config.Config, path string, dryRun bool) ([]string, error) {
	// Only settings reach the config file, so only a committed settings
	// kind keeps the absolute path out of it.
	if cfg == nil || !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) || !emit.PersonalMemoryLeavesCheckoutIn(cfg, path, []string{"settings"}, target) {
		return nil, nil
	}
	dir, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		return nil, err
	}
	if err := sess.CreateRepoMemoryStore(cfg, dir, dryRun); err != nil {
		return nil, err
	}
	return []string{"Write(" + filepath.ToSlash(dir) + "/**)"}, nil
}

// mergeMemoryAllow adds rules to permissions.allow. An allow list
// settings produce is sync's whole, so the rules join it. Otherwise the
// list is the user's: sync keeps their rules and claims only its own,
// and takes its earlier rules out once rules no longer holds them.
func mergeMemoryAllow(sess *emit.Session, keys map[string]any, path string, rules []string, dryRun bool) {
	permissions, _ := keys[permissionsKey].(map[string]any)
	if allow, ok := permissions["allow"].([]string); ok {
		for _, rule := range rules {
			if !slices.Contains(allow, rule) {
				allow = append(allow, rule)
			}
		}
		permissions["allow"] = allow
		return
	}
	if len(rules) == 0 {
		// A rule an earlier sync wrote goes with its claim.
		return
	}
	if sess.ClaimsUnchangedValue(path, dryRun, permissionsKey, "allow") {
		// The last sync wrote the whole list and nobody edited it since,
		// so this one replaces it.
		if permissions == nil {
			permissions = map[string]any{}
			keys[permissionsKey] = permissions
		}
		permissions["allow"] = slices.Clone(rules)
		return
	}
	existing, _ := sess.ExistingJSONObject(path, permissionsKey, dryRun)["allow"].([]any)
	planned := make([]any, len(rules))
	for i, rule := range rules {
		planned[i] = rule
	}
	// A list the last sync wrote whole and the user edited since stays
	// as it is, but the store rule in it is still sync's, so the item
	// claim below takes over from the whole one.
	if !emit.ClaimsWholeValue(path, permissionsKey, "allow") {
		planned = emit.WithoutUserItems(path, []string{permissionsKey, "allow"}, existing, planned)
	}
	if len(planned) == 0 {
		// The user already allows the store; the list stays theirs.
		return
	}
	if permissions == nil {
		permissions = map[string]any{}
		keys[permissionsKey] = permissions
	}
	merged, claims := emit.MergeOwnedLists(path, []string{permissionsKey},
		map[string][]any{"allow": existing},
		map[string][]any{"allow": planned})
	permissions["allow"] = emit.ClaimedJSONItems(merged["allow"], claims["allow"])
}
