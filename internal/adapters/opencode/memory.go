package opencode

import (
	"encoding/json"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
// permission.external_directory. OpenCode applies the last matching
// rule, so the entries keep the order they were written in and the store
// patterns come last.
//
// A `permission` map settings produce is sync's whole, so the patterns
// join it, after the spec's own entries in the spec's order. Otherwise
// the map is the user's: sync sets only external_directory, keeps the
// user's entries in file order, and claims only its own. It reports
// whether `permission` merges one level deep.
func mergeExternalDirectories(sess *emit.Session, keys map[string]any, settings []spec.Entry, path string, patterns []string, dryRun bool) (bool, error) {
	if permissions, ok := keys[permissionKey].(map[string]any); ok {
		value := permissions[externalDirectoryKey]
		if _, object := value.(map[string]any); !object && len(patterns) == 0 {
			return false, nil
		}
		rules, err := specRules(value, settings)
		if err != nil {
			return false, err
		}
		permissions[externalDirectoryKey], err = withAllowed(rules, patterns)
		return false, err
	}
	if len(patterns) == 0 {
		// A store entry an earlier sync wrote goes with its claim.
		return false, nil
	}
	if emit.ClaimsWholeValue(path, permissionKey) {
		// The last sync wrote the whole map, so this one replaces it.
		rules, err := withAllowed(emit.NewOrderedJSON(), patterns)
		keys[permissionKey] = map[string]any{externalDirectoryKey: rules}
		return false, err
	}
	keyPath := []string{permissionKey, externalDirectoryKey}
	rules := sess.ExistingObjectAt(path, keyPath, dryRun)
	if rules == nil {
		rules = emit.NewOrderedJSON()
		if action, ok := sess.ExistingJSONObject(path, permissionKey, dryRun)[externalDirectoryKey].(string); ok {
			if err := rules.Set(catchAllPattern, action); err != nil {
				return false, err
			}
		}
	}
	prior := emit.PriorClaimedEntries(path, keyPath)
	for _, pattern := range prior {
		if isAllow(rules, pattern) && !slices.Contains(patterns, pattern) {
			rules.Delete(pattern)
		}
	}
	var owned []string
	for _, pattern := range patterns {
		if _, set := rules.Get(pattern); set && !slices.Contains(prior, pattern) {
			// The user's own rule for the store stays theirs, where it is.
			continue
		}
		owned = append(owned, pattern)
	}
	rules, err := withAllowed(rules, owned)
	if err != nil {
		return false, err
	}
	permission := map[string]any{externalDirectoryKey: emit.ClaimedJSONEntries(rules, owned)}
	if action, ok := existingAction(sess, path, dryRun); ok {
		// A bare action sets every permission. Its object form is the
		// catch-all key, which stays the user's and leads the object.
		permission[catchAllPattern] = emit.CarriedJSONValue(action)
	}
	keys[permissionKey] = permission
	return true, nil
}

// existingAction returns the `permission` value on disk when it is a
// bare action, such as "deny", rather than an object.
func existingAction(sess *emit.Session, path string, dryRun bool) (string, bool) {
	raw, found := sess.ExistingObjectAt(path, nil, dryRun).Get(permissionKey)
	var action string
	return action, found && json.Unmarshal(raw, &action) == nil
}

// specRules returns the external_directory value settings produced as an
// ordered pattern object: a pattern map in the order the last spec that
// sets it writes it, or a bare action as the catch-all pattern.
func specRules(value any, settings []spec.Entry) (*emit.OrderedJSON, error) {
	switch v := value.(type) {
	case string:
		rules := emit.NewOrderedJSON()
		return rules, rules.Set(catchAllPattern, v)
	case map[string]any:
		var order []string
		for _, entry := range settings {
			if _, ok := nativePermission(entry)[externalDirectoryKey]; ok {
				order = entry.KeyOrder("x-"+target, permissionKey, externalDirectoryKey)
			}
		}
		return emit.OrderedObject(v, order)
	}
	return emit.NewOrderedJSON(), nil
}

// withAllowed moves each pattern to the end of rules, allowed, so it
// wins over every rule before it.
func withAllowed(rules *emit.OrderedJSON, patterns []string) (*emit.OrderedJSON, error) {
	for _, pattern := range patterns {
		rules.Delete(pattern)
		if err := rules.Set(pattern, "allow"); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func isAllow(rules *emit.OrderedJSON, pattern string) bool {
	raw, ok := rules.Get(pattern)
	var action string
	return ok && json.Unmarshal(raw, &action) == nil && action == "allow"
}
