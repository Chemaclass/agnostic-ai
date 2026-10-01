package cli

import (
	"fmt"
	"maps"
	"slices"
)

// nextModelAliases is the alias record a sync writes: what it resolved
// for each target it emitted, and the previous record for the rest.
func nextModelAliases(prev, resolved map[string]map[string]string, emitted []string) map[string]map[string]string {
	next := map[string]map[string]string{}
	for target, aliases := range prev {
		if !slices.Contains(emitted, target) {
			next[target] = aliases
		}
	}
	for _, target := range emitted {
		if len(resolved[target]) > 0 {
			next[target] = resolved[target]
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

// addedModelAliases is prev plus each alias an emitted target resolved
// that prev does not record yet. A moved alias keeps its old id.
func addedModelAliases(prev, resolved map[string]map[string]string, emitted []string) map[string]map[string]string {
	next := map[string]map[string]string{}
	for target, aliases := range prev {
		next[target] = maps.Clone(aliases)
	}
	for _, target := range emitted {
		for alias, model := range resolved[target] {
			if _, ok := next[target][alias]; ok {
				continue
			}
			if next[target] == nil {
				next[target] = map[string]string{}
			}
			next[target][alias] = model
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

// movedAliasNotes names each alias that now resolves to another id than
// the previous sync recorded, sorted by target and alias.
func movedAliasNotes(prev, resolved map[string]map[string]string) []string {
	var notes []string
	for _, target := range slices.Sorted(maps.Keys(resolved)) {
		for _, alias := range slices.Sorted(maps.Keys(resolved[target])) {
			was, ok := prev[target][alias]
			if now := resolved[target][alias]; ok && was != now {
				notes = append(notes, fmt.Sprintf("%s: %s now resolves to %s (was %s)", target, alias, now, was))
			}
		}
	}
	return notes
}
