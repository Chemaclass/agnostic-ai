package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

var permissionLists = []string{"allow", "deny", "ask"}

// PermissionLists names the rule lists an OwnedRules record covers.
func PermissionLists() []string { return slices.Clone(permissionLists) }

// OwnedRules is the record of the permission rules the last sync added
// to a settings file that merges into the file on disk. Without it a
// removed rule would stay.
type OwnedRules struct {
	path  string
	owned map[string][]string
	// exists is true when the record or a legacy one is on disk.
	exists bool
	// recorded is true when the record at path exists. A legacy record
	// may hold only some of the rules sync wrote.
	recorded bool
}

// ReadOwnedRules loads the record at path, or else the first legacy
// record found. Record always writes path.
func ReadOwnedRules(path string, legacy ...string) (OwnedRules, error) {
	o := OwnedRules{path: path}
	for _, candidate := range append([]string{path}, legacy...) {
		raw, err := os.ReadFile(candidate)
		if IsAbsent(err) {
			continue
		}
		if err != nil {
			return o, fmt.Errorf("%s: %w", candidate, err)
		}
		if err := json.Unmarshal(raw, &o.owned); err != nil {
			return o, fmt.Errorf("parse %s: %w", candidate, err)
		}
		o.exists = true
		o.recorded = candidate == path
		break
	}
	return o, nil
}

// Exists reports whether a record, current or legacy, is on disk.
func (o OwnedRules) Exists() bool { return o.exists }

// Recorded reports whether the current record is on disk.
func (o OwnedRules) Recorded() bool { return o.recorded }

// Strip returns base without the rules the last sync recorded.
func (o OwnedRules) Strip(base map[string]any) map[string]any {
	if base == nil || len(o.owned) == 0 {
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

// Record writes the rules of final that sync generated and keep does
// not hold. keep is the user's own rules; a nil keep adopts every
// generated rule already on disk, so removing it from the spec later
// removes it from the file.
func (o OwnedRules) Record(sess *Session, final, keep map[string]any, generated []map[string]any, dryRun bool) error {
	owned := OwnedRulesOf(final, keep, generated)
	if !o.exists && len(owned) == 0 {
		return nil
	}
	body, err := json.MarshalIndent(owned, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", o.path, err)
	}
	return sess.WriteFile(o.path, string(body)+"\n", dryRun)
}

// OwnedRulesOf returns, per list, the rules of final that sync generated
// and keep does not hold: what Record writes.
func OwnedRulesOf(final, keep map[string]any, generated []map[string]any) map[string][]string {
	owned := map[string][]string{}
	for _, list := range permissionLists {
		made := map[string]bool{}
		for _, layer := range generated {
			for _, rule := range stringRules(layer[list]) {
				made[rule] = true
			}
		}
		user := stringRules(keep[list])
		for _, rule := range stringRules(final[list]) {
			if made[rule] && !slices.Contains(user, rule) && !slices.Contains(owned[list], rule) {
				owned[list] = append(owned[list], rule)
			}
		}
	}
	return owned
}

func stringRules(raw any) []string {
	var out []string
	switch list := raw.(type) {
	case []any:
		for _, rule := range list {
			if s, ok := rule.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, list...)
	}
	return out
}
