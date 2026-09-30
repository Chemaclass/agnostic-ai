package spec

import (
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ApplyModelTiers rewrites each agent, skill, command, and settings spec
// whose `model` names a tier in tiers, as a string or as the per-target
// map's `default`, into the tier's per-target map. Entries the spec's own
// map sets win over the tier's. The tier's effort applies only when the
// spec sets no `effort`. Every other value, including `model.<target>`
// and `x-<target>.model`, stays a literal model id.
func (b *Bundle) ApplyModelTiers(tiers map[string]config.ModelTier) {
	if len(tiers) == 0 {
		return
	}
	for _, entries := range [][]Entry{b.Agents, b.Skills, b.Commands, b.Settings} {
		for i := range entries {
			entries[i].applyModelTier(tiers)
		}
	}
}

func (e *Entry) applyModelTier(tiers map[string]config.ModelTier) {
	var name string
	var own map[string]any
	switch model := e.Meta["model"].(type) {
	case string:
		name = model
	case map[string]any:
		name, _ = model["default"].(string)
		own = model
	}
	tier, ok := tiers[name]
	if !ok {
		return
	}
	resolved := make(map[string]any, len(tier.Models)+len(own))
	for target, model := range tier.Models {
		resolved[target] = model
	}
	for target, model := range own {
		if target != "default" {
			resolved[target] = model
		}
	}
	e.Meta = maps.Clone(e.Meta)
	e.Meta["model"] = resolved
	e.ModelTier = name
	if _, set := e.Meta["effort"]; set || tier.Effort == nil {
		return
	}
	e.Meta["effort"] = cloneEffort(tier.Effort)
	if at := slices.Index(e.MetaKeys, "model"); at >= 0 && !slices.Contains(e.MetaKeys, "effort") {
		e.MetaKeys = slices.Insert(slices.Clone(e.MetaKeys), at+1, "effort")
	}
}

func cloneEffort(effort any) any {
	if m, ok := effort.(map[string]any); ok {
		return maps.Clone(m)
	}
	return effort
}
