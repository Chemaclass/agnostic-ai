package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProtectedRulesFile records the permission rules sync added to
// settings.json for protected paths. settings.json merges into what is
// on disk, so without it a rule would outlive the block that wrote it.
const ProtectedRulesFile = ".agnostic-ai-protected.json"

// protectedRules tracks the Edit rules sync owns in settings.json. A
// rule already on disk before sync first protected its path belongs to
// the user and is never recorded.
type protectedRules struct {
	path   string
	rules  map[string]any
	owned  map[string][]string
	active bool
}

func readProtectedRules(dir string, settings []spec.Entry) (protectedRules, error) {
	p := protectedRules{path: filepath.Join(dir, ProtectedRulesFile), rules: protectedPermissions(settings)}
	raw, err := os.ReadFile(p.path)
	if err != nil && !emit.IsAbsent(err) {
		return p, fmt.Errorf("%s: %w", p.path, err)
	}
	if err == nil {
		if err := json.Unmarshal(raw, &p.owned); err != nil {
			return p, fmt.Errorf("parse %s: %w", p.path, err)
		}
	}
	p.active = len(p.rules) > 0 || err == nil
	return p, nil
}

// strip returns base without the rules the last sync recorded.
func (p protectedRules) strip(base map[string]any) map[string]any {
	if !p.active || base == nil {
		return base
	}
	out := make(map[string]any, len(base))
	for key, value := range base {
		owned := p.owned[key]
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

// record writes the protected rules the stripped base does not already
// hold, which are the ones this sync adds.
func (p protectedRules) record(sess *emit.Session, base map[string]any, dryRun bool) error {
	if !p.active {
		return nil
	}
	owned := map[string][]string{}
	for list, raw := range p.rules {
		existing, _ := base[list].([]any)
		for _, rule := range raw.([]any) {
			if !slices.Contains(existing, rule) {
				owned[list] = append(owned[list], rule.(string))
			}
		}
	}
	body, err := json.MarshalIndent(owned, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", p.path, err)
	}
	return sess.WriteFile(p.path, string(body)+"\n", dryRun)
}
