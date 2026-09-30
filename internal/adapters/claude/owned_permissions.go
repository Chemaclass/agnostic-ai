package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// OwnedPermissionsFile records the permission rules sync added to
// settings.json: portable, protected, config, and x-claude rules.
// settings.json merges into the file on disk when no overlay was
// captured, so without the record a removed rule would stay.
const OwnedPermissionsFile = ".agnostic-ai-permissions.json"

// LegacyProtectedRulesFile is the earlier record, which held only the
// rules generated from protected paths.
const LegacyProtectedRulesFile = ".agnostic-ai-protected.json"

var permissionLists = []string{"allow", "deny", "ask"}

// ownedPermissions is the record of the rules the last sync added.
type ownedPermissions struct {
	path  string
	owned map[string][]string
	// exists is true when either record is on disk.
	exists bool
	// recorded is true when the full record exists. Without it, sync
	// cannot tell its earlier rules from the user's.
	recorded bool
}

func readOwnedPermissions(dir string) (ownedPermissions, error) {
	o := ownedPermissions{path: filepath.Join(dir, OwnedPermissionsFile)}
	for _, name := range []string{OwnedPermissionsFile, LegacyProtectedRulesFile} {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if emit.IsAbsent(err) {
			continue
		}
		if err != nil {
			return o, fmt.Errorf("%s: %w", path, err)
		}
		if err := json.Unmarshal(raw, &o.owned); err != nil {
			return o, fmt.Errorf("parse %s: %w", path, err)
		}
		o.exists = true
		o.recorded = name == OwnedPermissionsFile
		break
	}
	return o, nil
}

// strip returns base without the rules the last sync recorded.
func (o ownedPermissions) strip(base map[string]any) map[string]any {
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

// record writes the rules of final that sync generated and keep does
// not hold. keep is the user's own rules: the captured overlay, or the
// on-disk rules left after strip. On the first sync without a record,
// keep is nil, so every generated rule already on disk is adopted:
// removing it from the spec later removes it from settings.json.
func (o ownedPermissions) record(sess *emit.Session, final, keep map[string]any, generated []map[string]any, dryRun bool) error {
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
	if !o.exists && len(owned) == 0 {
		return nil
	}
	body, err := json.MarshalIndent(owned, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", o.path, err)
	}
	return sess.WriteFile(o.path, string(body)+"\n", dryRun)
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
