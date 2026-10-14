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

// RepoMemoryFiles names opencode.json, which holds the repo store of
// personal memory once memory.personal is repo.
func (Adapter) RepoMemoryFiles(cfg *config.Config) []string {
	path := emit.OutputMCPFile(cfg, target, defaultMCPFile)
	if !emit.RepoMemoryFile(cfg, path, nil, target) {
		return nil
	}
	return []string{path}
}

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
	if !emit.PersonalMemoryGrantable(target, dir, "OpenCode will ask before it writes memory there") {
		return nil, nil
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
// A `permission` map settings produce is sync's for the tools they
// name, and the patterns join external_directory, after the spec's own
// entries in the spec's order. Otherwise the map is the user's: sync
// sets only external_directory, keeps the user's entries in file order,
// and claims only its own. Either way a tool the user wrote and no spec
// sets stays, and each store entry is also claimed on its own, so it
// leaves by itself, and only while unchanged, once no longer wanted. It
// reports whether `permission` merges one level deep, which is always
// when it is written.
func mergeExternalDirectories(sess *emit.Session, keys map[string]any, settings []spec.Entry, path string, patterns, kept []string, dryRun bool) (bool, error) {
	if permissions, ok := keys[permissionKey].(map[string]any); ok {
		if _, specSets := permissions[externalDirectoryKey]; !specSets {
			// No spec sets external_directory, so it stays the user's and
			// sync adds only its store entries.
			value, wanted, err := userDirectoryRules(sess, path, patterns, kept, dryRun)
			if err != nil {
				return false, err
			}
			if wanted {
				permissions[externalDirectoryKey] = value
			}
			carryBareAction(sess, permissions, path, dryRun)
			keys[permissionKey] = permissions
			return true, nil
		}
		value := permissions[externalDirectoryKey]
		onDisk := emit.NewOrderedJSON()
		if len(kept) > 0 {
			if existing := sess.ExistingObjectAt(path, []string{permissionKey, externalDirectoryKey}, dryRun); existing != nil {
				onDisk = existing
			}
		}
		kept = slices.DeleteFunc(slices.Clone(kept), func(pattern string) bool {
			_, set := onDisk.Get(pattern)
			return !set
		})
		var carried []string
		if _, object := value.(map[string]any); object || len(patterns) > 0 || len(kept) > 0 {
			rules, err := specRules(value, settings)
			if err != nil {
				return false, err
			}
			// A refused store entry rides along in the value sync rewrites,
			// as it stands. It stays sync's only if sync wrote it and
			// nobody changed it.
			for _, pattern := range kept {
				if _, set := rules.Get(pattern); set {
					continue
				}
				raw, _ := onDisk.Get(pattern)
				var action any
				if err := json.Unmarshal(raw, &action); err != nil {
					return false, err
				}
				if err := rules.Set(pattern, action); err != nil {
					return false, err
				}
				if action == "allow" && slices.Contains(emit.PriorClaimedEntries(path, []string{permissionKey, externalDirectoryKey}), pattern) {
					carried = append(carried, pattern)
				}
			}
			// A pattern the spec already sets is the spec's, whatever its action.
			patterns = slices.DeleteFunc(slices.Clone(patterns), func(pattern string) bool {
				_, set := rules.Get(pattern)
				return set
			})
			if permissions[externalDirectoryKey], err = withAllowed(rules, patterns); err != nil {
				return false, err
			}
		}
		// Each tool is claimed on its own, so the merge keeps the tools
		// the user wrote there and the specs do not produce.
		if value, ok := permissions[externalDirectoryKey]; ok {
			entries := [][]string{}
			for _, pattern := range append(slices.Clone(patterns), carried...) {
				entries = append(entries, []string{pattern})
			}
			permissions[externalDirectoryKey] = emit.ClaimedJSONWithEntries(value, entries)
		}
		carryBareAction(sess, permissions, path, dryRun)
		keys[permissionKey] = permissions
		return true, nil
	}
	value, ok, err := userDirectoryRules(sess, path, patterns, kept, dryRun)
	if err != nil || !ok {
		return false, err
	}
	permission := map[string]any{externalDirectoryKey: value}
	carryBareAction(sess, permission, path, dryRun)
	keys[permissionKey] = permission
	return true, nil
}

// userDirectoryRules returns the external_directory value for a `permission`
// whose external_directory no spec sets: the user's own entries in file
// order, with only the store entries sync adds claimed as its own. It
// reports false when there is no store entry to write.
func userDirectoryRules(sess *emit.Session, path string, patterns, kept []string, dryRun bool) (any, bool, error) {
	// A refused store entry sync wrote itself stays sync's while it is unchanged.
	claimed := emit.PriorClaimedEntries(path, []string{permissionKey, externalDirectoryKey})
	for _, pattern := range kept {
		if slices.Contains(claimed, pattern) && !slices.Contains(patterns, pattern) {
			patterns = append(slices.Clone(patterns), pattern)
		}
	}
	if len(patterns) == 0 {
		// A store entry an earlier sync claimed goes with its claim.
		return nil, false, nil
	}
	keyPath := []string{permissionKey, externalDirectoryKey}
	// A map the last sync wrote whole from a spec that is gone now goes
	// in this write while nobody has edited it (the nested merge drops
	// it), and stays as the user's once edited.
	rules := emit.NewOrderedJSON()
	if !sess.ClaimsUnchangedValue(path, dryRun, permissionKey) && !sess.ClaimsUnchangedValue(path, dryRun, permissionKey, externalDirectoryKey) {
		if existing := sess.ExistingObjectAt(path, keyPath, dryRun); existing != nil {
			rules = existing
		} else if action, ok := sess.ExistingJSONObject(path, permissionKey, dryRun)[externalDirectoryKey].(string); ok {
			if err := rules.Set(catchAllPattern, action); err != nil {
				return nil, false, err
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
		if _, set := rules.Get(pattern); set && (!slices.Contains(prior, pattern) || !isAllow(rules, pattern)) {
			// The user's own rule for the store stays theirs, where it is,
			// and so does a rule they edited after sync wrote it.
			continue
		}
		owned = append(owned, pattern)
	}
	// A claimed entry stays where it is, so a broader rule the user put
	// after it keeps winning; only a missing one goes last.
	rules, err := withAllowed(rules, slices.DeleteFunc(slices.Clone(owned), func(pattern string) bool {
		_, set := rules.Get(pattern)
		return set
	}))
	if err != nil {
		return nil, false, err
	}
	return emit.ClaimedJSONEntries(rules, owned), true, nil
}

// carryBareAction keeps a bare action on disk, such as "deny", which sets
// every permission, as the catch-all key that leads the object.
func carryBareAction(sess *emit.Session, permission map[string]any, path string, dryRun bool) {
	if action, ok := existingAction(sess, path, dryRun); ok {
		if _, set := permission[catchAllPattern]; !set {
			permission[catchAllPattern] = emit.CarriedJSONValue(action)
		}
	}
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
