package overview

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func resolvedField(t *testing.T, in Inputs, key string) map[string]TargetValue {
	t.Helper()
	out := map[string]TargetValue{}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		if f.Key != key {
			continue
		}
		for _, r := range f.Resolved {
			out[r.Target] = r
		}
	}
	return out
}

func TestResolveTargets_TierClearAndUnset(t *testing.T) {
	tiers := map[string]config.ModelTier{"fast": {Models: map[string]string{"claude": "haiku"}, Effort: "low"}}
	team := entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"model": "opus"})
	cleared := entry(spec.KindSettings, "team", ".agnostic-ai/local/settings/team.yaml", map[string]any{})
	cleared.Layer, cleared.Base, cleared.LocalMeta = "project-user", &team, map[string]any{"model": nil}
	tiered := entry(spec.KindSettings, "tiered", ".agnostic-ai/settings/tiered.yaml", map[string]any{"model": "fast"})
	b := spec.Bundle{Settings: []spec.Entry{cleared, tiered}}
	b.ApplyModelTiers(tiers)
	in := Inputs{Cfg: &config.Config{Targets: []string{"claude", "codex"}, Models: tiers}, Bundle: b, Compared: []string{"claude"}}

	model := resolvedField(t, in, "model")
	claude := model["claude"]
	if claude.Value != "haiku" || claude.Tier != "fast" || claude.From == nil || claude.From.Path != ".agnostic-ai/settings/tiered.yaml" || !claude.Checked {
		t.Errorf("claude model = %+v, want haiku from tier fast", claude)
	}
	if len(claude.Others) == 0 || !strings.Contains(claude.Others[0].Reason, "clears") || claude.Others[0].Value != "null" {
		t.Errorf("claude others = %+v, want the personal clear", claude.Others)
	}
	codex := model["codex"]
	if codex.Value != "" || codex.From != nil || codex.Checked {
		t.Errorf("codex model = %+v, want unset and unchecked", codex)
	}
	if len(codex.Others) != 2 || !strings.Contains(codex.Others[1].Reason, "no value for Codex") {
		t.Errorf("codex others = %+v, want the tier map without codex", codex.Others)
	}
	if effort := resolvedField(t, in, "effort")["claude"]; effort.Value != "low" || effort.Tier != "fast" {
		t.Errorf("claude effort = %+v, want low from tier fast", effort)
	}
}

func TestResolveTargets_ConfigReplacesTheSettingsValueAndNotesExplainDrops(t *testing.T) {
	cfg := &config.Config{Targets: []string{"codex", "cursor"}, Outputs: map[string]config.Output{"codex": {Config: &config.CodexConfig{ModelReasoningEffort: "xhigh"}}}}
	in := Inputs{Cfg: cfg, Compared: []string{"codex", "cursor"}, Bundle: spec.Bundle{Settings: []spec.Entry{
		entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"effort": "high"}),
	}}, FieldNotes: []FieldNote{{Target: "cursor", Field: "effort", Reason: "the settings file has no repository effort key"}}}

	effort := resolvedField(t, in, "effort")
	codex := effort["codex"]
	if codex.Value != "xhigh" || codex.From == nil || codex.From.Path != "outputs.codex.config.model-reasoning-effort" || len(codex.Others) != 1 || codex.Others[0].Value != "high" {
		t.Errorf("codex effort = %+v, want the config value over the spec", codex)
	}
	if cursor := effort["cursor"]; cursor.Value != "high" || cursor.Dropped != "the settings file has no repository effort key" {
		t.Errorf("cursor effort = %+v, want the drop note", cursor)
	}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		if (f.Key == "model" || f.Key == "effort") != (len(f.Resolved) == 2) {
			t.Errorf("%s resolved = %+v", f.Key, f.Resolved)
		}
	}
}
