package opencode

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// externalDirectoryKey is the `permission` key for tool calls that touch
// paths outside the directory OpenCode started in. It defaults to "ask"
// (opencode.ai/docs/permissions).
const externalDirectoryKey = "external_directory"

// memoryDirectoryRules returns the external_directory pattern for the
// repo store of personal memory when instructions name that store, so
// OpenCode saves there without asking each time.
func memoryDirectoryRules(sess *emit.Session, cfg *config.Config, path string, dryRun bool) ([]string, error) {
	if cfg == nil || !slices.Contains(cfg.Builtins, emit.MemoryBuiltin) || !emit.PersonalMemoryLeavesCheckout(cfg, path, target) {
		return nil, nil
	}
	dir, err := emit.PersonalMemoryDirFor(cfg, path, target)
	if err != nil {
		return nil, err
	}
	if err := sess.CreateRepoMemoryStore(cfg, dir, dryRun); err != nil {
		return nil, err
	}
	return []string{filepath.ToSlash(dir) + "/**"}, nil
}

// mergeExternalDirectories allows the memory store patterns under
// permission.external_directory. A `permission` map settings produce is
// sync's whole, so the patterns join it. Otherwise the map is the user's:
// sync sets only external_directory, keeps the user's entries there, and
// claims only its own. It reports whether `permission` merges one level
// deep.
func mergeExternalDirectories(sess *emit.Session, keys map[string]any, path string, patterns []string, dryRun bool) bool {
	if permissions, ok := keys[permissionKey].(map[string]any); ok {
		if len(patterns) > 0 {
			permissions[externalDirectoryKey] = withAllowed(permissions[externalDirectoryKey], patterns)
		}
		return false
	}
	if len(patterns) == 0 {
		// A store entry an earlier sync wrote goes with its claim.
		return false
	}
	if emit.ClaimsWholeValue(path, permissionKey) {
		// The last sync wrote the whole map, so this one replaces it.
		keys[permissionKey] = map[string]any{externalDirectoryKey: withAllowed(nil, patterns)}
		return false
	}
	keyPath := []string{permissionKey, externalDirectoryKey}
	existing := sess.ExistingJSONObject(path, permissionKey, dryRun)[externalDirectoryKey]
	rules := withAllowed(existing, nil)
	prior := emit.PriorClaimedEntries(path, keyPath)
	for _, pattern := range prior {
		if rules[pattern] == "allow" && !slices.Contains(patterns, pattern) {
			delete(rules, pattern)
		}
	}
	var owned []string
	for _, pattern := range patterns {
		if _, set := rules[pattern]; set && !slices.Contains(prior, pattern) {
			// The user's own rule for the store stays theirs.
			continue
		}
		rules[pattern] = "allow"
		owned = append(owned, pattern)
	}
	keys[permissionKey] = map[string]any{externalDirectoryKey: emit.ClaimedJSONEntries(rules, owned)}
	return true
}

// withAllowed returns an external_directory value as a pattern map with
// patterns allowed. A bare action covers every path, so it becomes the
// catch-all pattern, which sorts first and so yields to the patterns.
func withAllowed(value any, patterns []string) map[string]any {
	rules := map[string]any{}
	switch v := value.(type) {
	case string:
		rules[catchAllPattern] = v
	case map[string]any:
		maps.Copy(rules, v)
	}
	for _, pattern := range patterns {
		rules[pattern] = "allow"
	}
	return rules
}
