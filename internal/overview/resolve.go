package overview

import (
	"fmt"
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
func resolveTargets(in Inputs, key string, labels labeler) []TargetValue {
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
			if !hasField(e, key) {
				if e.Base != nil && LocalSets(e, key) && hasField(*e.Base, key) && e.EmitsTo(target) {
					tv.Others = append(tv.Others, ValueOrigin{Spec: e.Name, Path: SpecPath(root, e), Local: true, Value: "null",
						Reason: "Your personal file clears this field, so the value in " + SettingsTeamPath(root, e) + " does not apply."})
				}
				continue
			}
			origin := ValueOrigin{Spec: e.Name, Path: SettingsTeamPath(root, e), Value: settingsValueFor(e, key, target)}
			if LocalSets(e, key) {
				origin.Path, origin.Local = SpecPath(root, e), true
				if base := baseValue(e, key, target, in.Cfg.Models); base != "" {
					tv.Others = append(tv.Others, ValueOrigin{Spec: e.Name, Path: SettingsTeamPath(root, e), Value: base,
						Reason: "Your personal file " + origin.Path + " overrides it."})
				}
			}
			if key == "model" {
				origin.Tier = e.ModelTier
			} else if tierEffort(e, target, in.Cfg.Models) == origin.Value && origin.Value != "" {
				origin.Tier = e.ModelTier
			}
			switch {
			case !e.EmitsTo(target):
				origin.Reason = "This spec does not apply to " + label + "."
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
		}
		out = append(out, tv)
	}
	return out
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
	if e.Base == nil || !hasField(*e.Base, key) {
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
		if n.Target == target && n.Field == field && !slices.Contains(reasons, n.Reason) {
			reasons = append(reasons, n.Reason)
		}
	}
	return strings.Join(reasons, "; ")
}
