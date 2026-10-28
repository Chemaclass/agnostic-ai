package overview

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// resolvedFields are the settings fields whose winning value the view
// explains for each configured tool.
var resolvedFields = []string{"model", "effort"}

// resolveTargets explains, for each configured target, the value sync
// writes for key from the settings specs, with the specs that lose.
// Each spec resolves through the same functions the adapters call.
func resolveTargets(in Inputs, key string, labels labeler, caps map[string][]spec.Kind) []TargetValue {
	if in.Cfg == nil || !slices.Contains(resolvedFields, key) {
		return nil
	}
	root := in.Snapshot.Project.Root
	out := make([]TargetValue, 0, len(in.Cfg.Targets))
	for _, target := range in.Cfg.Targets {
		label := labels.of(target)
		tv := TargetValue{Target: target, Label: label, Checked: slices.Contains(in.Compared, target), Others: []ValueOrigin{}}
		var winners []ValueOrigin
		for _, e := range in.Bundle.Settings {
			if custom, ok := e.Meta["x-"+target].(map[string]any); ok && key == "model" && custom["model"] != nil && e.EmitsTo(target) {
				tv.Native = append(tv.Native, ValueOrigin{Spec: e.Name, Path: SpecPath(root, e), Value: fmt.Sprint(custom["model"])})
			}
			base := baseValue(e, key, target, in.Cfg.Models)
			if !hasField(e, key) {
				if base != "" && LocalSets(e, key) && e.EmitsTo(target) {
					tv.Others = append(tv.Others, ValueOrigin{Spec: e.Name, Path: SpecPath(root, e), Local: true, Value: "null",
						Reason: "Your personal file clears " + base + " from " + SettingsTeamPath(root, e) + "."})
				}
				continue
			}
			origin := ValueOrigin{Spec: e.Name, Path: SettingsTeamPath(root, e), Value: settingsValueFor(e, key, target)}
			if key == "model" {
				origin.Tier = e.ModelTier
			} else if e.ModelTier != "" && tierEffort(e, target, in.Cfg.Models) == origin.Value && origin.Value != "" {
				origin.Tier = e.ModelTier
			}
			fromTier := key == "effort" && origin.Tier != ""
			if LocalSets(e, key) && !fromTier || e.Base == nil && LocalLayer(e) {
				if e.Base == nil || base != origin.Value {
					origin.Path, origin.Local = SpecPath(root, e), true
				}
				if base != "" && base != origin.Value {
					tv.Others = append(tv.Others, ValueOrigin{Spec: e.Name, Path: SettingsTeamPath(root, e), Value: base,
						Reason: "Your personal file " + origin.Path + " overrides it."})
				}
			}
			origin.PerTool = !fromTier && perToolValue(e, key, target, origin.Local, in.Cfg.Models)
			foreign := key == "model" && !in.Global && foreignClaudeModel(e, target)
			if foreign {
				origin.Value = settingsValueFor(withoutSharedModel(e), key, target)
			}
			switch {
			case !e.EmitsTo(target):
				origin.Reason = "This spec does not apply to " + label + "."
				tv.Others = append(tv.Others, origin)
			case origin.Value == "" && foreign:
				origin.Value = settingsValueFor(e, key, target)
				origin.Reason = "It is a Claude model name, which " + label + " cannot load, so sync leaves it out."
				tv.Others = append(tv.Others, origin)
			case origin.Value == "":
				origin.Reason = "Its per-tool map has no value for " + label + " and no default."
				tv.Others = append(tv.Others, origin)
			default:
				winners = append(winners, origin)
			}
		}
		if len(winners) > 0 {
			won := winners[len(winners)-1]
			for _, lost := range winners[:len(winners)-1] {
				lost.Reason = won.Path + " loads later and sets it too."
				tv.Others = append(tv.Others, lost)
			}
			tv.From = &won
			tv.Value, tv.Tier = won.Value, won.Tier
			if key == "model" {
				tv.Alias = aliasOf(in.Bundle.Settings, won, target)
			}
		}
		if path, value := configOverride(in.Cfg, key, target); value != "" {
			if tv.From != nil {
				lost := *tv.From
				lost.Reason = path + " in the config replaces it."
				tv.Others = append(tv.Others, lost)
			}
			tv.From = &ValueOrigin{Path: path, Value: value}
			tv.Value, tv.Tier, tv.Alias = value, "", ""
		} else if tv.From != nil {
			tv.Dropped = in.droppedReason(target, key)
			if kinds, known := caps[target]; known && !slices.Contains(kinds, spec.KindSettings) {
				tv.Dropped = label + " does not take settings specs"
			}
		}
		out = append(out, tv)
	}
	return out
}

// perToolValue reports whether e gives target its value for key from
// the target's own entry in the per-tool map. Tiers expand into per-tool
// entries, so an entry that matches the tier counts as the tier's.
func perToolValue(e spec.Entry, key, target string, local bool, tiers map[string]config.ModelTier) bool {
	meta := e.Meta
	if e.Base != nil {
		meta = e.Base.Meta
		if local {
			meta = e.LocalMeta
		}
	}
	m, ok := meta[key].(map[string]any)
	if !ok {
		return false
	}
	v, ok := m[target]
	if !ok {
		return false
	}
	if tier, tiered := tiers[e.ModelTier]; key == "model" && tiered && e.Base == nil {
		return tier.Models[target] != fmt.Sprint(v)
	}
	return true
}

// foreignClaudeModel reports whether e's shared model is a Claude model
// name target cannot load, which project sync drops before it resolves.
// sync --global writes it.
func foreignClaudeModel(e spec.Entry, target string) bool {
	return adapters.ForeignClaudeModel(target, adapters.SharedModel(map[string]any{"model": e.Meta["model"]}, target))
}

// withoutSharedModel is e without the shared part of its model, as the
// adapters drop a foreign Claude model name.
func withoutSharedModel(e spec.Entry) spec.Entry {
	meta := maps.Clone(e.Meta)
	if model, ok := meta["model"].(map[string]any); ok && len(model) > 1 {
		model = maps.Clone(model)
		delete(model, "default")
		meta["model"] = model
	} else {
		delete(meta, "model")
	}
	e.Meta = meta
	return e
}

func settingsValueFor(e spec.Entry, key, target string) string {
	if key == "model" {
		return adapters.SettingsModel([]spec.Entry{e}, target)
	}
	if v := adapters.SettingsEffort([]spec.Entry{e}, target); v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// baseValue is the value the team spec a personal file extends gives
// key, with its model tier applied.
func baseValue(e spec.Entry, key, target string, tiers map[string]config.ModelTier) string {
	if e.Base == nil {
		return ""
	}
	b := spec.Bundle{Settings: []spec.Entry{*e.Base}}
	b.ApplyModelTiers(tiers)
	return settingsValueFor(b.Settings[0], key, target)
}

// tierEffort is the effort e's model tier gives target, or "".
func tierEffort(e spec.Entry, target string, tiers map[string]config.ModelTier) string {
	tier, ok := tiers[e.ModelTier]
	if e.ModelTier == "" || !ok || tier.Effort == nil {
		return ""
	}
	return settingsValueFor(spec.Entry{Meta: map[string]any{"effort": tier.Effort}}, "effort", target)
}

func aliasOf(settings []spec.Entry, won ValueOrigin, target string) string {
	for _, e := range settings {
		if e.Name == won.Spec {
			return adapters.ModelAliasIn(e.Meta, target)
		}
	}
	return ""
}

// configOverride is the agnostic-ai.yaml key that replaces the settings
// value of key on target, and its value, as the adapter writes it.
func configOverride(cfg *config.Config, key, target string) (string, string) {
	out := cfg.Outputs[target]
	switch {
	case target == "claude" && key == "model" && out.Settings != nil:
		return "outputs.claude.settings.model", out.Settings.Model
	case target == "codex" && key == "model" && out.Config != nil:
		return "outputs.codex.config.model", out.Config.Model
	case target == "codex" && key == "effort" && out.Config != nil:
		return "outputs.codex.config.model-reasoning-effort", out.Config.ModelReasoningEffort
	}
	return "", ""
}

func (in Inputs) droppedReason(target, field string) string {
	var reasons []string
	for _, n := range in.FieldNotes {
		// resolveTargets already skips a foreign Claude model name, as project sync does.
		if strings.Contains(n.Reason, "is a Claude model name") {
			continue
		}
		if n.Target == target && n.Field == field && !slices.Contains(reasons, n.Reason) {
			reasons = append(reasons, n.Reason)
		}
	}
	return strings.Join(reasons, "; ")
}
