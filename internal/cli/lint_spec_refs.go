package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintSpecRefs flags a {{$AGENT:<name>}} or {{$SKILL:<name>}} that names
// no agent or skill in the project, or one that does not reach a target
// the referring spec reaches, by its own targets or the target's kinds (LINT033, error). Sync would still render a
// phrase there, which points the model at nothing.
func lintSpecRefs(targets []string, support kindSupport, b spec.Bundle) []lintFinding {
	known := map[string][]string{
		adapters.RefAgent: sortedSpecNames(b.Agents),
		adapters.RefSkill: sortedSpecNames(b.Skills),
	}
	byName := map[string]map[string]spec.Entry{adapters.RefAgent: {}, adapters.RefSkill: {}}
	for _, a := range b.Agents {
		byName[adapters.RefAgent][a.Name] = a
	}
	for _, sk := range b.Skills {
		byName[adapters.RefSkill][sk.Name] = sk
	}
	var out []lintFinding
	for _, e := range b.All() {
		reported := map[string]bool{}
		for _, ref := range adapters.BodyRefs(e.Body) {
			names := known[ref.Keyword]
			if reported[ref.Token] {
				continue
			}
			kind := "agent"
			if ref.Keyword == adapters.RefSkill {
				kind = "skill"
			}
			if named, ok := byName[ref.Keyword][ref.Name]; ok {
				var missing []string
				for _, t := range targets {
					_, syncsKind := support[named.Kind][t]
					if e.EmitsTo(t) && (!named.EmitsTo(t) || !syncsKind) && strings.Contains(spec.FilterFences(e.Body, []string{t}), ref.Token) {
						missing = append(missing, t)
					}
				}
				if len(missing) > 0 {
					reported[ref.Token] = true
					out = append(out, lintFinding{
						Code:     "LINT033",
						Severity: lintError,
						Path:     e.Path,
						Message:  fmt.Sprintf("%s reaches %s, where %s %q does not sync; scope this spec or the %s to the same targets", ref.Token, strings.Join(missing, ", "), kind, ref.Name, kind),
					})
				}
				continue
			}
			reported[ref.Token] = true
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
