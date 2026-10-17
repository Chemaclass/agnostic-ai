package windsurf

import (
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// RepoMemoryFiles names the Devin config, whose permissions.allow holds
// the repo store of personal memory once memory.personal is repo.
func (Adapter) RepoMemoryFiles(cfg *config.Config) []string {
	path := emit.OutputConfFile(cfg, target, defaultConfigFile)
	if !emit.RepoMemoryFile(cfg, path, []string{"settings"}, target) {
		return nil
	}
	return []string{path}
}

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
	if !emit.PersonalMemoryGrantable(target, dir, "Devin will ask before it writes memory there") {
		return nil, nil
	}
	if err := sess.CreateRepoMemoryStore(cfg, dir, dryRun); err != nil {
		return nil, err
	}
	return []string{writeRule(dir)}, nil
}

// writeRule is the permissions.allow rule that lets Devin write under dir.
func writeRule(dir string) string {
	return "Write(" + filepath.ToSlash(dir) + "/**)"
}

// mergePermissionLists merges the lists settings produce into
// permissions.allow, .deny, and .ask, with rules, the store rules sync
// adds, joining allow. Each list is the user's: sync keeps their rules
// in file order and claims only its own, so a rule leaves by itself, and
// only while unchanged, once no spec or store wants it. kept holds a
// refused handoff rule already in the file.
func mergePermissionLists(sess *emit.Session, keys map[string]any, path string, rules, kept []string, dryRun bool) {
	permissions, _ := keys[permissionsKey].(map[string]any)
	for _, list := range permissionLists {
		var planned, stay []string
		if values, ok := permissions[list].([]string); ok {
			planned = slices.Clone(values)
		}
		if list == "allow" {
			planned = append(planned, rules...)
			stay = kept
		}
		merged, ok := mergePermissionList(sess, path, list, planned, stay, dryRun)
		if !ok {
			delete(permissions, list)
			continue
		}
		if permissions == nil {
			permissions = map[string]any{}
			keys[permissionsKey] = permissions
		}
		permissions[list] = merged
	}
	if len(permissions) == 0 {
		delete(keys, permissionsKey)
	}
}

// mergePermissionList returns the merge value for one rule list, or
// false when nothing is planned for it and its earlier claims should
// release.
func mergePermissionList(sess *emit.Session, path, list string, planned, kept []string, dryRun bool) (any, bool) {
	keyPath := []string{permissionsKey, list}
	// A refused rule sync wrote itself stays sync's while it is unchanged.
	planned = append(planned, emit.StillClaimedItems(path, keyPath, kept)...)
	if len(planned) == 0 {
		return nil, false
	}
	// A list the last sync wrote whole goes while nobody has edited it,
	// and stays as the user's once edited. The item claims below take
	// over from the whole one.
	var existing []any
	if !sess.ClaimsUnchangedValue(path, dryRun, keyPath...) {
		existing, _ = sess.ExistingJSONObject(path, permissionsKey, dryRun)[list].([]any)
	}
	wanted := make([]any, 0, len(planned))
	seen := map[string]bool{}
	for _, rule := range planned {
		if !seen[rule] {
			seen[rule] = true
			wanted = append(wanted, rule)
		}
	}
	// An edited list an earlier version claimed whole holds sync's rules
	// among the user's, and a rule the specs still plan is sync's: the
	// claims move to one per rule.
	if !emit.ClaimsWholeValue(path, keyPath...) {
		wanted = emit.WithoutUserItems(path, keyPath, existing, wanted)
	}
	merged, claims := emit.MergeOwnedLists(path, []string{permissionsKey},
		map[string][]any{list: existing},
		map[string][]any{list: wanted})
	rest := withSurplusCopies(existing, merged[list], wanted, emit.PriorClaimedItems(path, keyPath))
	for _, rule := range kept {
		if !slices.Contains(rest, any(rule)) && !slices.Contains(planned, rule) {
			rest = append(rest, rule)
		}
	}
	if len(rest) == 0 {
		return nil, false
	}
	if len(claims[list]) == 0 {
		// Sync adds nothing the user does not hold, so an earlier claim
		// on the list has nothing left to name.
		return emit.CarriedJSONValue(rest), true
	}
	return emit.ClaimedJSONItems(rest, claims[list]), true
}

// withSurplusCopies puts back, in file order, every copy of a rule sync
// owns beyond the last one, which is the one it wrote: merged is the user's entries followed by
// wanted, which MergeOwnedLists built after dropping each copy of a rule
// sync owns.
func withSurplusCopies(existing, merged, wanted []any, prior []string) []any {
	user := slices.Clone(merged[:len(merged)-len(wanted)])
	owned := map[string]bool{}
	for _, rule := range wanted {
		owned[emit.ContentSum(rule.(string))] = true
	}
	for _, sum := range prior {
		owned[sum] = true
	}
	// Sync appends its own copy last, so that is the one it owns.
	last := map[string]int{}
	for i, entry := range existing {
		if rule, ok := entry.(string); ok && owned[emit.ContentSum(rule)] {
			last[emit.ContentSum(rule)] = i
		}
	}
	var out []any
	for i, entry := range existing {
		sum := ""
		if rule, ok := entry.(string); ok {
			sum = emit.ContentSum(rule)
		}
		switch at := slices.Index(user, entry); {
		case owned[sum] && last[sum] == i:
		case owned[sum]:
			out = append(out, entry)
		case at >= 0:
			out = append(out, entry)
			user = slices.Delete(user, at, at+1)
		}
	}
	return append(out, merged[len(merged)-len(wanted):]...)
}
