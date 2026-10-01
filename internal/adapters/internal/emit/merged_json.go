package emit

import (
	"encoding/json"
	"fmt"
	"os"
)

// carriedJSONValue is a merge value sync writes without claiming it.
type carriedJSONValue struct{ value any }

// CarriedJSONValue, as a key's value in a merge, sets the key without
// recording it as one sync wrote. Use it for a value read back from the
// file, such as the user's entries a cleanup keeps, so releasing the
// file later does not delete them.
func CarriedJSONValue(value any) any {
	return carriedJSONValue{value: value}
}

func uncarried(value any) (any, bool) {
	if c, ok := value.(carriedJSONValue); ok {
		return c.value, false
	}
	return value, true
}

// WriteMergedJSON is WriteFile for a JSON file that also holds keys sync
// did not write. keys lists the key paths sync set, so a later sync that
// stops writing the file can take out only those (ReleaseMergedJSON).
// released lists the paths this write removed or left to the user, which
// an earlier sync may have set.
func (s *Session) WriteMergedJSON(path, content string, keys, released [][]string, dryRun bool) error {
	s.mu.Lock()
	mark := len(s.detailed)
	s.mu.Unlock()
	if err := s.WriteFile(path, content, dryRun); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := mark; i < len(s.detailed); i++ {
		if s.detailed[i].Path == path {
			s.detailed[i].Merged = true
			s.detailed[i].Keys = keys
			s.detailed[i].Released = released
		}
	}
	return nil
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
)

// ReleaseMergedJSON takes the key paths sync set out of the merged JSON
// file at path and keeps every other key. The file goes away only when
// sync created it and nothing else is left. A missing file is
// MergedUnchanged.
func (s *Session) ReleaseMergedJSON(path string, keys [][]string, created, dryRun bool) (MergedRelease, error) {
	if s.skipUnmanaged(path) {
		return MergedKept, nil
	}
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return MergedUnchanged, nil
	}
	if err != nil {
		return MergedKept, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := s.readExistingJSON(path, false)
	if err != nil {
		return MergedKept, nil
	}
	changed := false
	for _, key := range keys {
		changed = deleteJSONPath(doc, key) || changed
	}
	if doc.Len() == 0 && created {
		if _, err := s.remove(path, "", existing, false, dryRun); err != nil {
			return MergedKept, err
		}
		return MergedRemoved, nil
	}
	if !changed {
		return MergedUnchanged, nil
	}
	raw, err := MarshalJSONIndentWith(doc, DetectJSONIndent(existing))
	if err != nil {
		return MergedKept, fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := s.WriteFile(path, string(raw)+"\n", dryRun); err != nil {
		return MergedKept, err
	}
	return MergedStripped, nil
}

// deleteJSONPath removes the value at path, then every object on the
// way that the removal left empty. It reports whether doc changed.
func deleteJSONPath(doc *OrderedJSON, path []string) bool {
	if len(path) == 0 {
		return false
	}
	raw, found := doc.Get(path[0])
	if !found {
		return false
	}
	if len(path) == 1 {
		doc.Delete(path[0])
		return true
	}
	child := NewOrderedJSON()
	if err := json.Unmarshal(raw, child); err != nil || !deleteJSONPath(child, path[1:]) {
		return false
	}
	if child.Len() == 0 {
		doc.Delete(path[0])
		return true
	}
	return doc.Set(path[0], child) == nil
}
