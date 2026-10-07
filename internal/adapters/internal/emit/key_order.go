package emit

import "sort"

// ExistingObjectAt returns the object at keyPath in the JSON or JSONC
// file at path in file order, or nil when the file or a key on the way
// is missing or holds something else.
func (s *Session) ExistingObjectAt(path string, keyPath []string, dryRun bool) *OrderedJSON {
	return s.existingObject(path, keyPath, dryRun)
}

// OrderedObject returns object with its keys in order, then any key order
// leaves out, sorted.
func OrderedObject(object map[string]any, order []string) (*OrderedJSON, error) {
	out := NewOrderedJSON()
	for _, key := range order {
		value, ok := object[key]
		if _, done := out.Get(key); !ok || done {
			continue
		}
		if err := out.Set(key, value); err != nil {
			return nil, err
		}
	}
	rest := make([]string, 0, len(object))
	for key := range object {
		if _, set := out.Get(key); !set {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		if err := out.Set(key, object[key]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
