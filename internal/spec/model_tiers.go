package spec

import (
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ApplyModelTiers rewrites each agent, skill, command, and settings spec
// whose `model` names a tier in tiers, as a string or as the per-target
// map's `default`, into the tier's per-target map. Entries the spec's own
// map sets win over the tier's. The tier's effort applies only when the
// spec sets no `effort`, and only to targets whose model the tier gives. Every other value, including `model.<target>`
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
	resolved := make(map[string]any, len(tier.Models))
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
	effort := tier.Effort
	shared := true
	if m, ok := effort.(map[string]any); ok {
		m = maps.Clone(m)
		_, shared = m["default"]
		effort = m
	}
	for _, target := range e.ownModelTargets(own) {
		if m, ok := effort.(map[string]any); ok {
			delete(m, target)
		}
		if shared {
			e.dropTargetEffort(target)
		}
	}
	e.Meta["effort"] = effort
	if at := slices.Index(e.MetaKeys, "model"); at >= 0 && !slices.Contains(e.MetaKeys, "effort") {
		e.MetaKeys = slices.Insert(slices.Clone(e.MetaKeys), at+1, "effort")
	}
}

// ownModelTargets lists the targets whose model the spec sets itself,
// under `model.<target>` or `x-<target>.model`. The tier's effort does
// not reach them, since it was chosen for the tier's model.
func (e *Entry) ownModelTargets(own map[string]any) []string {
	var targets []string
	for target := range own {
		if target != "default" {
			targets = append(targets, target)
		}
	}
	for key, value := range e.Meta {
		target, ok := strings.CutPrefix(key, "x-")
		if custom, isMap := value.(map[string]any); ok && isMap {
			if _, set := custom["model"]; set && !slices.Contains(targets, target) {
				targets = append(targets, target)
			}
		}
	}
	return targets
}

// dropTargetEffort writes the `x-<target>.effort: null` delete marker so a
// shared tier effort skips target, unless the spec sets one there.
func (e *Entry) dropTargetEffort(target string) {
	custom, _ := e.Meta["x-"+target].(map[string]any)
	if _, set := custom["effort"]; set {
		return
	}
	custom = maps.Clone(custom)
	if custom == nil {
		custom = map[string]any{}
	}
	custom["effort"] = nil
	e.Meta["x-"+target] = custom
}
