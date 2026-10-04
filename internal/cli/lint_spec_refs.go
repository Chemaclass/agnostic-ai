package cli

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintSpecRefs flags a {{$AGENT:<name>}} or {{$SKILL:<name>}} that names
// no agent or skill in the project (LINT033, error). Sync would still
// render a phrase for it, which points the model at nothing.
func lintSpecRefs(b spec.Bundle) []lintFinding {
	known := map[string][]string{
		adapters.RefAgent: sortedSpecNames(b.Agents),
		adapters.RefSkill: sortedSpecNames(b.Skills),
	}
	var out []lintFinding
	for _, e := range b.All() {
		reported := map[string]bool{}
		for _, ref := range adapters.BodyRefs(e.Body) {
			names := known[ref.Keyword]
			if slices.Contains(names, ref.Name) || reported[ref.Token] {
				continue
			}
			reported[ref.Token] = true
			kind := "agent"
			if ref.Keyword == adapters.RefSkill {
				kind = "skill"
			}
			have := "none"
			if len(names) > 0 {
				have = strings.Join(names, ", ")
			}
			out = append(out, lintFinding{
				Code:     "LINT033",
				Severity: lintError,
				Path:     e.Path,
				Message:  fmt.Sprintf("%s names no %s %q; known %ss: %s", ref.Token, kind, ref.Name, kind, have),
			})
		}
	}
	return out
}

func sortedSpecNames(entries []spec.Entry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}
