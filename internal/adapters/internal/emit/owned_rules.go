package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

// OwnedRules tracks the permission rules sync adds to a settings file
// that merges into what is on disk. Without the record a rule would
// outlive the spec that wrote it. A rule already on disk before sync
// first wrote it belongs to the user and is never recorded.
type OwnedRules struct {
	path   string
	rules  map[string][]string
	owned  map[string][]string
	active bool
}

// ReadOwnedRules loads the record at path for the rules, keyed by
// list, that this sync writes.
func ReadOwnedRules(path string, rules map[string][]string) (OwnedRules, error) {
	o := OwnedRules{path: path, rules: rules}
	raw, err := os.ReadFile(path)
	if err != nil && !IsAbsent(err) {
		return o, fmt.Errorf("%s: %w", path, err)
	}
	if err == nil {
		if err := json.Unmarshal(raw, &o.owned); err != nil {
			return o, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	o.active = len(rules) > 0 || err == nil
	return o, nil
}

// Active reports whether this sync writes rules or an earlier one did.
func (o OwnedRules) Active() bool { return o.active }

// Strip returns base without the rules the last sync recorded.
func (o OwnedRules) Strip(base map[string]any) map[string]any {
	if !o.active || base == nil {
		return base
	}
	out := make(map[string]any, len(base))
	for key, value := range base {
		owned := o.owned[key]
		list, ok := value.([]any)
		if !ok || len(owned) == 0 {
			out[key] = value
			continue
		}
		kept := slices.DeleteFunc(slices.Clone(list), func(rule any) bool {
			s, _ := rule.(string)
			return slices.Contains(owned, s)
		})
		if len(kept) > 0 {
			out[key] = kept
		}
	}
	return out
}

// Record writes the rules the stripped base does not already hold,
// which are the ones this sync adds.
func (o OwnedRules) Record(sess *Session, base map[string]any, dryRun bool) error {
	if !o.active {
		return nil
	}
	owned := map[string][]string{}
	for list, rules := range o.rules {
		existing, _ := base[list].([]any)
		for _, rule := range rules {
			if !slices.Contains(existing, any(rule)) {
				owned[list] = append(owned[list], rule)
			}
		}
	}
	body, err := json.MarshalIndent(owned, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", o.path, err)
	}
	return sess.WriteFile(o.path, string(body)+"\n", dryRun)
}
