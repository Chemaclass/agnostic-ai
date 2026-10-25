package overview

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSettingsFields_ExposeEditContract(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	fields := buildFields(SectionSettings, nil, nil, cfg, false)
	byKey := map[string]SettingField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if got := byKey["effort"].Editable; got == nil || got.Path != ".agnostic-ai/settings/defaults.yaml" || !slices.Contains(got.Allowed, "high") {
		t.Fatalf("effort metadata = %+v", got)
	}
	if got := byKey["session-retention-days"].Editable; got == nil || got.Kind != "number" || got.DisabledReason == "" {
		t.Fatalf("project retention = %+v", got)
	}
	fields = buildFields(SectionSettings, nil, nil, cfg, true)
	for _, f := range fields {
		if f.Key == "session-retention-days" && (f.Editable == nil || f.Editable.Path != "settings/defaults.yaml" || f.Editable.DisabledReason != "") {
			t.Fatalf("global retention = %+v", f.Editable)
		}
	}
}

func TestSettingsFields_EditTargetsTheSpecWhoseValueIsInEffect(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	in := Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{
		entry(spec.KindSettings, "a", ".agnostic-ai/settings/a.yaml", map[string]any{"name": "a", "model": "sonnet"}),
		entry(spec.KindSettings, "b", ".agnostic-ai/settings/b.yaml", map[string]any{"name": "b", "model": "opus"}),
		entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"name": "team", "permissions": map[string]any{"allow": []any{"Read"}}, "effort": "high"}),
	}}}
	byKey := map[string]SettingField{}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		byKey[f.Key] = f
	}
	model := byKey["model"].Editable
	if model == nil || model.Path != ".agnostic-ai/settings/b.yaml" || slices.Contains(model.Paths, ".agnostic-ai/settings/team.yaml") {
		t.Fatalf("model edit = %+v", model)
	}
	effort := byKey["effort"]
	if len(effort.Set) != 1 || effort.Set[0].Spec != "team" {
		t.Fatalf("effort set = %+v", effort.Set)
	}
	if effort.Editable == nil || effort.Editable.Path != ".agnostic-ai/settings/team.yaml" || !slices.Contains(effort.Editable.Paths, ".agnostic-ai/settings/team.yaml") {
		t.Fatalf("effort edit = %+v", effort.Editable)
	}
}
