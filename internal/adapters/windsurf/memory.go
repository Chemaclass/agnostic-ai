package windsurf

import (
	"path/filepath"
	"slices"
	"strings"

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
		// A rule sync claimed on its own goes with its claim.
		dropStoreRules(sess, keys, path, dryRun)
		return
	}
	// A list the last sync wrote whole from a spec that is gone now goes
	// while nobody has edited it, as a stale claim's release would take
	// it, and stays as the user's once edited. Either way the store rule
	// in it was sync's, and the item claim below takes over from the
	// whole one.
	var existing []any
	if !sess.ClaimsUnchangedValue(path, dryRun, permissionsKey, "allow") {
		existing, _ = sess.ExistingJSONObject(path, permissionsKey, dryRun)["allow"].([]any)
	}
	planned := make([]any, len(rules))
	for i, rule := range rules {
		planned[i] = rule
	}
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

// dropStoreRules takes the store rules sync wrote out of an allow list
// the last sync wrote whole and the user has edited since, once no store
// rule is wanted. That list stays the user's, so its whole claim cannot
// take the rule out. An unchanged list goes whole on its own.
func dropStoreRules(sess *emit.Session, keys map[string]any, path string, dryRun bool) {
	if !emit.ClaimsWholeValue(path, permissionsKey, "allow") || sess.ClaimsUnchangedValue(path, dryRun, permissionsKey, "allow") {
		return
	}
	existing, _ := sess.ExistingJSONObject(path, permissionsKey, dryRun)["allow"].([]any)
	kept := slices.DeleteFunc(slices.Clone(existing), func(rule any) bool {
		text, _ := rule.(string)
		glob, ok := strings.CutPrefix(text, "Write(")
		glob, closed := strings.CutSuffix(glob, ")")
		return ok && closed && emit.IsRepoStoreGlob(glob)
	})
	if len(kept) == len(existing) {
		return
	}
	permissions, _ := keys[permissionsKey].(map[string]any)
	if permissions == nil {
		permissions = map[string]any{}
		keys[permissionsKey] = permissions
	}
	if len(kept) == 0 {
		permissions["allow"] = emit.RemoveJSONKey
		return
	}
	permissions["allow"] = emit.CarriedJSONValue(kept)
}
