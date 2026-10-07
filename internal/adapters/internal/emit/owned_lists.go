package emit

import (
	"encoding/json"
	"slices"
)

// MergeOwnedLists merges planned into onDisk, a map of lists the user
// shares with sync, such as hook entries per event. Under each key it
// keeps the entries sync did not write, drops the ones the last sync
// claimed in the merged file at path under keyPath, adopts a user entry
// equal to a planned one instead of writing it twice, and appends the
// planned entries. A key left empty is dropped. claims holds each key's
// planned entries for ClaimedJSONItems.
//
// A file an earlier version claimed whole at keyPath counts as all
// sync's while it is unchanged, so upgrading does not keep stale entries.
func MergeOwnedLists(path string, keyPath []string, onDisk, planned map[string][]any) (merged map[string][]any, claims map[string][]string) {
	prior := priorMergedKeys(path)
	priorItems := map[string][]string{}
	for key := range onDisk {
		claim, _ := priorClaim(prior, append(slices.Clone(keyPath), key))
		priorItems[key] = claim.Items
	}
	var wholeOwned bool
	if whole, err := json.Marshal(onDisk); err == nil {
		wholeOwned = unchangedSince(prior, keyPath, whole)
	}
	return mergeOwnedLists(priorItems, wholeOwned, onDisk, planned)
}

func mergeOwnedLists(prior map[string][]string, wholeOwned bool, onDisk, planned map[string][]any) (map[string][]any, map[string][]string) {
	merged := map[string][]any{}
	claims := map[string][]string{}
	for key, entries := range onDisk {
		var plannedSums []string
		for _, entry := range planned[key] {
			plannedSums = append(plannedSums, ContentSum(canonicalJSON(entry)))
		}
		for _, entry := range entries {
			sum := ContentSum(canonicalJSON(entry))
			if wholeOwned || slices.Contains(prior[key], sum) || slices.Contains(plannedSums, sum) {
				continue
			}
			merged[key] = append(merged[key], entry)
		}
	}
	for key, entries := range planned {
		for _, entry := range entries {
			merged[key] = append(merged[key], entry)
			claims[key] = append(claims[key], canonicalJSON(entry))
		}
	}
	for key, entries := range merged {
		if len(entries) == 0 {
			delete(merged, key)
		}
	}
	return merged, claims
}

// canonicalJSON is entry as JSON with sorted keys, the form a claimed
// item's sum is taken from.
func canonicalJSON(entry any) string {
	switch v := entry.(type) {
	case string:
		return v
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return string(v)
		}
		entry = decoded
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	return string(raw)
}

// RawEventLists decodes a hooks object into its event lists, each entry
// kept as raw JSON so its key order survives the merge.
func RawEventLists(raw []byte) (map[string][]any, error) {
	var events map[string][]json.RawMessage
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, err
	}
	out := make(map[string][]any, len(events))
	for event, entries := range events {
		for _, entry := range entries {
			out[event] = append(out[event], entry)
		}
	}
	return out, nil
}

// OwnedEventLists returns the merge value for the object of lists at key,
// such as `hooks`, in the JSON file at path, for a merge that goes one
// level into key: each list holds the entries on disk sync did not write
// plus planned's, with only sync's entries claimed, and a list left empty
// is removed. ok is false with no planned entries: the merge then leaves
// the key alone, and releasing sync's claims takes out only its entries.
func (s *Session) OwnedEventLists(path, key string, planned any, dryRun bool) (value any, ok bool) {
	children, ok := s.ownedLists(path, []string{key}, planned, dryRun)
	if !ok {
		return nil, false
	}
	return children, true
}

// OwnedRootLists is OwnedEventLists for a file whose top-level keys hold
// the lists, such as Factory's `hooks.json`. It returns the merge keys.
func (s *Session) OwnedRootLists(path string, planned any, dryRun bool) (keys map[string]any, ok bool) {
	children, ok := s.ownedLists(path, nil, planned, dryRun)
	if !ok {
		return nil, false
	}
	return children.values, true
}

func (s *Session) ownedLists(path string, keyPath []string, planned any, dryRun bool) (orderedChildren, bool) {
	onDisk := map[string][]any{}
	existing := s.existingObject(path, keyPath, dryRun)
	if existing != nil {
		raw, err := MarshalJSONCompact(existing)
		if err != nil {
			return orderedChildren{}, false
		}
		if onDisk, err = RawEventLists(raw); err != nil {
			return orderedChildren{}, false
		}
	}
	plannedLists := map[string][]any{}
	if planned != nil && !isNilOrdered(planned) {
		raw, err := MarshalJSONCompact(planned)
		if err != nil {
			return orderedChildren{}, false
		}
		if plannedLists, err = RawEventLists(raw); err != nil {
			return orderedChildren{}, false
		}
	}
	if len(plannedLists) == 0 {
		// Releasing the claims takes out only sync's entries.
		return orderedChildren{}, false
	}
	merged, claims := MergeOwnedLists(path, keyPath, onDisk, plannedLists)
	out := orderedChildren{values: map[string]any{}}
	var diskOrder []string
	if existing != nil {
		diskOrder = existing.Keys()
	}
	for _, event := range append(diskOrder, keyOrder(planned)...) {
		if _, done := out.values[event]; done {
			continue
		}
		entries, kept := merged[event]
		_, wasOnDisk := onDisk[event]
		switch {
		case !kept && !wasOnDisk:
			continue
		case !kept:
			out.values[event] = RemoveJSONKey
		case len(claims[event]) > 0:
			out.values[event] = ClaimedJSONItems(entries, claims[event])
		default:
			out.values[event] = CarriedJSONValue(entries)
		}
		out.order = append(out.order, event)
	}
	return out, true
}

// existingObject reads the object at keyPath, the whole document when
// keyPath is empty, from the JSON file at path, in file order.
func (s *Session) existingObject(path string, keyPath []string, dryRun bool) *OrderedJSON {
	doc, err := s.readExistingJSON(path, dryRun)
	if err != nil {
		return nil
	}
	if len(keyPath) == 0 {
		return doc
	}
	raw, found := jsonValueAt(doc, keyPath)
	object := NewOrderedJSON()
	if !found || json.Unmarshal(raw, object) != nil {
		return nil
	}
	return object
}

// keyOrder lists an object's keys: in order for an OrderedJSON, sorted
// for a map.
func keyOrder(value any) []string {
	switch v := value.(type) {
	case *OrderedJSON:
		return v.Keys()
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return keys
	}
	return nil
}

// orderedChildren is a merge value that sets the named children of an
// object key in order, each child its own merge value, and keeps the
// object's other children.
type orderedChildren struct {
	order  []string
	values map[string]any
}

func isNilOrdered(value any) bool {
	v, ok := value.(*OrderedJSON)
	return ok && v == nil
}

// ObjectAt decodes the object at key in the JSON document raw, in order.
func ObjectAt(raw []byte, key string) (*OrderedJSON, error) {
	doc := NewOrderedJSON()
	if err := json.Unmarshal(raw, doc); err != nil {
		return nil, err
	}
	value, found := doc.Get(key)
	object := NewOrderedJSON()
	if !found {
		return object, nil
	}
	if err := json.Unmarshal(value, object); err != nil {
		return nil, err
	}
	return object, nil
}

// ClaimsItemsUnder reports whether the last sync claimed entries one by
// one in the lists under key in the merged file at path, as
// OwnedEventLists does, rather than the whole key as older versions did.
func ClaimsItemsUnder(path, key string) bool {
	return slices.ContainsFunc(priorMergedKeys(path), func(k MergedKey) bool {
		return len(k.Path) == 2 && k.Path[0] == key && k.Items != nil
	})
}
