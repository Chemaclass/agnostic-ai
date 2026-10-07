package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

// MergedKey is one value sync set in a JSON file it merges into.
type MergedKey struct {
	Path []string `json:"path"`
	// Sum fingerprints the value sync wrote, so a value the user edited
	// since stays when the file is released.
	Sum string `json:"sum,omitempty"`
	// Items names sync's entries in a list it shares with the user, by
	// content sum once written, so the ledger holds no rule text.
	// Releasing the file takes out only these.
	Items []string `json:"items,omitempty"`
	// Follows, when set, is the sum of the value this write replaced. The
	// key is claimed only if an earlier sync's claim still has that sum;
	// otherwise the earlier claim stays as it was. It is never stored.
	Follows string `json:"-"`
}

// cleanedJSONValue is a merge value a cleanup rewrote from sync's own.
type cleanedJSONValue struct {
	value  any
	before string
}

// CleanedJSONValue, as a key's value in a merge, sets the key to after,
// a cleanup of before, the value on disk. When an earlier sync's claim
// still matches before, the user has not edited it, so the claim moves
// to after and releasing the file later takes after out whole.
// Otherwise it keeps whatever an earlier sync claimed, as
// KeptJSONValue does.
func CleanedJSONValue(before, after any) any {
	raw, err := json.Marshal(before)
	if err != nil {
		return KeptJSONValue(after)
	}
	return cleanedJSONValue{value: after, before: jsonValueSum(raw)}
}

// retiredJSONValue is a merge value read back from the file that no
// spec produces any more.
type retiredJSONValue struct {
	value  any
	before string
}

// RetiredJSONValue, as a key's value in a merge, stands for a value on
// disk, before, that no spec produces any more, with after the part to
// keep if it is not sync's. When an earlier sync's claim still matches
// before, the user has not edited it, so the key goes in this write.
// Otherwise after is set and the earlier claim kept, as KeptJSONValue
// does.
func RetiredJSONValue(before, after any) any {
	raw, err := json.Marshal(before)
	if err != nil {
		return KeptJSONValue(after)
	}
	return retiredJSONValue{value: after, before: jsonValueSum(raw)}
}

// carriedJSONValue is a merge value sync writes without claiming it.
type carriedJSONValue struct{ value any }

// CarriedJSONValue, as a key's value in a merge, sets the key without
// recording it as one sync wrote. Use it for a value read back from the
// file, such as the user's entries a cleanup keeps, so releasing the
// file later does not delete them.
func CarriedJSONValue(value any) any {
	return carriedJSONValue{value: value}
}

// keptJSONValue is a merge value that neither claims nor releases.
type keptJSONValue struct{ value any }

// KeptJSONValue, as a key's value in a merge, sets the key and keeps
// whatever an earlier sync claimed there, claiming nothing new. Use it
// for a value a cleanup rewrites from the file, whose entries may be
// sync's from before or the user's.
func KeptJSONValue(value any) any {
	return keptJSONValue{value: value}
}

// claimedItems is a list merge value of which sync owns only items.
type claimedItems struct {
	value any
	items []string
}

// ClaimedJSONItems, as a list value in a merge, sets the list and claims
// only items, the entries sync added beside the user's own. With no
// items it claims nothing new and keeps what an earlier sync claimed
// there: an entry sync added before is already in the list.
func ClaimedJSONItems(value any, items []string) any {
	return claimedItems{value: value, items: slices.Clone(items)}
}

// mergeClaimKind says how much of a merge value sync claims.
type mergeClaimKind int

const (
	claimWhole mergeClaimKind = iota
	claimItems
	claimNothing
	claimKeep
	claimFollow
	claimRetire
)

// mergeClaim unwraps a merge value and says how much of it sync claims.
// follows is the sum a claimFollow value's earlier claim must match.
func mergeClaim(value any) (unwrapped any, kind mergeClaimKind, items []string, follows string) {
	if v, ok := value.(cleanedJSONValue); ok {
		return v.value, claimFollow, nil, v.before
	}
	if v, ok := value.(retiredJSONValue); ok {
		return v.value, claimRetire, nil, v.before
	}
	unwrapped, kind, items = mergeClaimOf(value)
	return unwrapped, kind, items, ""
}

func mergeClaimOf(value any) (unwrapped any, kind mergeClaimKind, items []string) {
	switch v := value.(type) {
	case carriedJSONValue:
		return v.value, claimNothing, nil
	case keptJSONValue:
		return v.value, claimKeep, nil
	case claimedItems:
		if len(v.items) == 0 {
			return v.value, claimKeep, nil
		}
		return v.value, claimItems, v.items
	}
	return value, claimWhole, nil
}

// WriteMergedJSON is WriteFile for a JSON file that also holds keys sync
// did not write. keys lists the values sync set, so a later sync that
// stops writing the file can take out only those (ReleaseMergedJSON).
// released lists the key paths this write removed or left to the user,
// which an earlier sync may have set.
func (s *Session) WriteMergedJSON(path, content string, keys []MergedKey, released [][]string, dryRun bool) error {
	return s.writeMerged(path, content, withValueSums(content, keys), released, dryRun)
}

// writeMerged writes content and records keys, already summed, and
// released as the claims of a merged write.
func (s *Session) writeMerged(path, content string, keys []MergedKey, released [][]string, dryRun bool) error {
	s.mu.Lock()
	mark, captureMark := len(s.detailed), len(s.captured)
	if s.merging == nil {
		s.merging = map[string]bool{}
	}
	s.merging[path] = true
	s.mu.Unlock()
	err := s.WriteFile(path, content, dryRun)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.merging, path)
	if err != nil {
		return err
	}
	for i := mark; i < len(s.detailed); i++ {
		if s.detailed[i].Path == path {
			s.detailed[i].Merged = true
			s.detailed[i].Keys = keys
			s.detailed[i].Released = released
		}
	}
	for i := captureMark; i < len(s.captured); i++ {
		if s.captured[i].Path == path {
			s.captured[i].Merged = true
			s.captured[i].Keys = keys
			s.captured[i].Released = released
		}
	}
	return nil
}

// withValueSums fills in the sum of each whole value content holds.
func withValueSums(content string, keys []MergedKey) []MergedKey {
	doc := NewOrderedJSON()
	if err := json.Unmarshal([]byte(content), doc); err != nil {
		return keys
	}
	out := make([]MergedKey, 0, len(keys))
	for _, key := range keys {
		if key.Items != nil {
			key.Items = itemSums(key.Items)
		} else if raw, found := jsonValueAt(doc, key.Path); found {
			key.Sum = jsonValueSum(raw)
		}
		out = append(out, key)
	}
	return out
}

func itemSums(items []string) []string {
	sums := make([]string, len(items))
	for i, item := range items {
		sums[i] = ContentSum(item)
	}
	return sums
}

// jsonValueSum fingerprints a JSON value regardless of its formatting
// and key order.
func jsonValueSum(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ContentSum(string(raw))
	}
	return canonicalValueSum(value, string(raw))
}

// canonicalValueSum fingerprints a decoded value as canonical JSON, so
// the same value read from JSON or YAML sums the same. fallback is
// summed when the value has no JSON form.
func canonicalValueSum(value any, fallback string) string {
	canonical, err := json.Marshal(value)
	if err != nil {
		return ContentSum(fallback)
	}
	return ContentSum(string(canonical))
}

// MergedRelease says what ReleaseMergedJSON did with a file.
type MergedRelease int

const (
	// MergedKept left the file as it was: it is user-owned or does not
	// parse, so sync cannot tell its keys apart.
	MergedKept MergedRelease = iota
	// MergedUnchanged found none of sync's keys left in the file.
	MergedUnchanged
	// MergedStripped took sync's keys out and kept the rest.
	MergedStripped
	// MergedRemoved deleted a file sync created once nothing else was
	// left in it.
	MergedRemoved
	// MergedEdited took out sync's unedited keys and kept the ones the
	// user edited since sync wrote them.
	MergedEdited
)

// ReleaseMergedJSON takes the values sync set out of the merged JSON
// file at path and keeps every other key. A value whose sum no longer
// matches was edited by the user and stays, unless force is set; edited
// lists those. The file goes away only when sync created it and nothing
// else is left. A missing file is MergedUnchanged.
func (s *Session) ReleaseMergedJSON(path string, keys []MergedKey, created, force, dryRun bool) (result MergedRelease, edited []MergedKey, err error) {
	if s.skipUnmanaged(path) {
		return MergedKept, nil, nil
	}
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return MergedUnchanged, nil, nil
	}
	if err != nil {
		return MergedKept, nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := s.readExistingJSON(path, false)
	if err != nil {
		return MergedKept, nil, nil
	}
	changed := false
	for _, key := range keys {
		raw, found := jsonValueAt(doc, key.Path)
		switch {
		case !found:
		case key.Items != nil:
			changed = editJSONPath(doc, key.Path, func(raw json.RawMessage) (any, bool, bool) {
				return withoutItems(raw, key.Items)
			}) || changed
		case !force && key.Sum != "" && jsonValueSum(raw) != key.Sum:
			edited = append(edited, key)
		default:
			changed = editJSONPath(doc, key.Path, func(json.RawMessage) (any, bool, bool) { return nil, false, true }) || changed
		}
	}
	if len(edited) == 0 && doc.Len() == 0 && created {
		// A forced release takes out values the user edited, so backup mode
		// keeps the bytes, as for any file sync did not write itself.
		if _, err := s.remove(path, "", existing, force, dryRun); err != nil {
			return MergedKept, nil, err
		}
		return MergedRemoved, nil, nil
	}
	if changed {
		raw, err := MarshalJSONIndentWith(doc, DetectJSONIndent(existing))
		if err != nil {
			return MergedKept, nil, fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := s.WriteFile(path, string(raw)+"\n", dryRun); err != nil {
			return MergedKept, nil, err
		}
	}
	switch {
	case len(edited) > 0:
		return MergedEdited, edited, nil
	case changed:
		return MergedStripped, nil, nil
	}
	return MergedUnchanged, nil, nil
}

// withoutItems drops the entries whose sums are in items from the JSON
// list raw. It keeps the value unless the list ends up empty, and leaves
// anything but a list alone.
func withoutItems(raw json.RawMessage, items []string) (value any, keep, changed bool) {
	// Raw entries keep the user's key order in what stays.
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, true, false
	}
	kept := slices.DeleteFunc(slices.Clone(list), func(entry json.RawMessage) bool {
		return slices.Contains(items, ContentSum(canonicalJSON(entry)))
	})
	if len(kept) == len(list) {
		return nil, true, false
	}
	return kept, len(kept) > 0, true
}

// jsonValueAt returns the raw value at path in doc.
func jsonValueAt(doc *OrderedJSON, path []string) (json.RawMessage, bool) {
	if len(path) == 0 {
		return nil, false
	}
	raw, found := doc.Get(path[0])
	if !found || len(path) == 1 {
		return raw, found
	}
	child := NewOrderedJSON()
	if err := json.Unmarshal(raw, child); err != nil {
		return nil, false
	}
	return jsonValueAt(child, path[1:])
}

// editJSONPath replaces the value at path with what edit returns, or
// removes it when edit does not keep it, then removes every object on
// the way that the removal left empty. It reports whether doc changed.
func editJSONPath(doc *OrderedJSON, path []string, edit func(json.RawMessage) (value any, keep, changed bool)) bool {
	if len(path) == 0 {
		return false
	}
	raw, found := doc.Get(path[0])
	if !found {
		return false
	}
	if len(path) == 1 {
		value, keep, changed := edit(raw)
		switch {
		case !changed:
			return false
		case !keep:
			doc.Delete(path[0])
			return true
		}
		return doc.Set(path[0], value) == nil
	}
	child := NewOrderedJSON()
	if err := json.Unmarshal(raw, child); err != nil || !editJSONPath(child, path[1:], edit) {
		return false
	}
	if child.Len() == 0 {
		doc.Delete(path[0])
		return true
	}
	return doc.Set(path[0], child) == nil
}
