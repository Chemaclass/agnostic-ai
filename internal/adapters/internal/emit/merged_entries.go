package emit

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// entriesJSONValue is an object merge value merged entry by entry.
type entriesJSONValue struct{ entries map[string]any }

// MergeJSONEntries, as an object value in a merge, merges its entries
// by name into the object already in the file instead of replacing the
// object. An entry the user added stays. One sync wrote before and no
// longer sets goes, unless the user edited it since. Sync claims only
// its own entries. Use it for a map whose entries are whole records,
// such as MCP servers (#1552).
func MergeJSONEntries(entries map[string]any) any {
	return entriesJSONValue{entries: entries}
}

// MergeEntriesOf marks keys[key], when it is an object, to merge entry
// by entry (MergeJSONEntries). Call it after every other merge into
// keys, which expect plain values.
func MergeEntriesOf(keys map[string]any, key string) {
	if entries, ok := keys[key].(map[string]any); ok {
		keys[key] = MergeJSONEntries(entries)
	}
}

// PriorMergedKeys returns the values the last sync recorded for path in
// its ledger. The CLI sets it; nil means no record, as on a first sync.
var PriorMergedKeys func(path string) []MergedKey

func priorMergedKeys(path string) []MergedKey {
	if PriorMergedKeys == nil {
		return nil
	}
	return PriorMergedKeys(path)
}

// priorClaim returns the prior claim on keyPath.
func priorClaim(prior []MergedKey, keyPath []string) (MergedKey, bool) {
	i := slices.IndexFunc(prior, func(k MergedKey) bool { return slices.Equal(k.Path, keyPath) })
	if i < 0 {
		return MergedKey{}, false
	}
	return prior[i], true
}

// unchangedSince reports whether raw still holds the value the prior
// claim on keyPath recorded.
func unchangedSince(prior []MergedKey, keyPath []string, raw json.RawMessage) bool {
	claim, ok := priorClaim(prior, keyPath)
	return ok && claim.Items == nil && claim.Sum != "" && jsonValueSum(raw) == claim.Sum
}

// mergeJSONEntries returns the object at key in doc with entries merged
// in by name, and the paths of the entries sync now claims. An entry an
// earlier sync wrote, whole map or by name, that entries no longer
// holds goes while it is unchanged. A spec entry that replaces one the
// user wrote is noted. An entry JSON cannot encode, such as one holding a
// YAML .nan, fails the merge instead of being left out.
func (s *Session) mergeJSONEntries(path string, doc *OrderedJSON, key string, entries map[string]any) (*OrderedJSON, [][]string, error) {
	prior := priorMergedKeys(path)
	merged := NewOrderedJSON()
	raw, found := doc.Get(key)
	if found && json.Unmarshal(raw, merged) != nil {
		merged = NewOrderedJSON()
	}
	wholeOwned := found && unchangedSince(prior, []string{key}, raw)
	for _, name := range merged.Keys() {
		if _, wanted := entries[name]; wanted {
			continue
		}
		entryRaw, _ := merged.Get(name)
		if wholeOwned || unchangedSince(prior, []string{key, name}, entryRaw) {
			merged.Delete(name)
		}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	claimed := make([][]string, 0, len(names))
	for _, name := range names {
		entryPath := []string{key, name}
		if entryRaw, had := merged.Get(name); had && !s.IsCapturing() {
			_, synced := priorClaim(prior, entryPath)
			_, wholeClaim := priorClaim(prior, []string{key})
			if !synced && !wholeClaim && !sameJSONValue(entryRaw, entries[name]) {
				_, _ = fmt.Fprintf(Warner, "%s: the %q entry under %s comes from a spec now and replaces the one already there\n", path, name, key)
			}
		}
		if err := merged.Set(name, entries[name]); err != nil {
			return nil, nil, fmt.Errorf("%s: %s entry %q has a value JSON cannot hold: %w", path, key, name, err)
		}
		claimed = append(claimed, entryPath)
	}
	return merged, claimed, nil
}

func sameJSONValue(raw json.RawMessage, value any) bool {
	encoded, err := json.Marshal(value)
	return err == nil && jsonValueSum(raw) == jsonValueSum(encoded)
}
