package overview

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSettingsFields_ExposeEditContract(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	fields := buildFields(SectionSettings, nil, nil, cfg, "", false)
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
	fields = buildFields(SectionSettings, nil, nil, cfg, "", true)
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

func TestSettingsFields_TargetedAndPackSpecsDoNotTakeTheEdit(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	pack := entry(spec.KindSettings, "pack", ".agnostic-ai/packs/team/settings/pack.yaml", map[string]any{"name": "pack", "effort": "high"})
	pack.Layer = "pack:team"
	in := Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{
		pack,
		entry(spec.KindSettings, "a", ".agnostic-ai/settings/a.yaml", map[string]any{"name": "a", "model": "sonnet"}),
		entry(spec.KindSettings, "z", ".agnostic-ai/settings/z.yaml", map[string]any{"name": "z", "targets": []any{"codex"}, "model": "gpt-5"}),
	}}}
	byKey := map[string]SettingField{}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		byKey[f.Key] = f
	}
	if got := byKey["model"].Editable; got == nil || got.Path != ".agnostic-ai/settings/a.yaml" {
		t.Fatalf("model edit = %+v", got)
	}
	if got := byKey["effort"].Editable; got == nil || slices.Contains(got.Paths, pack.Path) || got.Path == pack.Path {
		t.Fatalf("effort edit = %+v", got)
	}
}

func TestSettingsFields_DefaultPathAndSyncCommandFollowTheScope(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "settings")
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: outside}}
	for _, f := range buildFields(SectionSettings, nil, nil, cfg, root, false) {
		if f.Key == "model" && (f.Editable.Path != filepath.ToSlash(filepath.Join(outside, "defaults.yaml")) || f.Editable.SyncCommand != "agnostic-ai sync") {
			t.Fatalf("project model = %+v", f.Editable)
		}
	}
	for _, f := range buildFields(SectionSettings, nil, nil, cfg, root, true) {
		if f.Key == "model" && f.Editable.SyncCommand != "agnostic-ai sync --global" {
			t.Fatalf("global model = %+v", f.Editable)
		}
	}
}

func TestSettingsFields_PackValueLaterInOrderPointsAtTheDefaultSpec(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	pack := entry(spec.KindSettings, "model", ".agnostic-ai/packs/team/settings/model.yaml", map[string]any{"name": "model", "model": "sonnet"})
	pack.Layer = "pack:team"
	in := Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{
		entry(spec.KindSettings, "base", ".agnostic-ai/settings/base.yaml", map[string]any{"name": "base", "effort": "high"}),
		pack,
	}}}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		if f.Key == "model" && (f.Editable == nil || f.Editable.Path != ".agnostic-ai/settings/defaults.yaml") {
			t.Fatalf("model edit = %+v", f.Editable)
		}
	}
}

func TestSettingsFields_NewDefaultSpecIsAChoiceAndNeverShadows(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	pack := entry(spec.KindSettings, "model", ".agnostic-ai/packs/team/settings/model.yaml", map[string]any{"name": "model", "model": "sonnet"})
	pack.Layer = "pack:team"
	editOf := func(settings ...spec.Entry) *FieldEdit {
		for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: settings}}), SectionSettings).Fields {
			if f.Key == "model" {
				return f.Editable
			}
		}
		return nil
	}
	got := editOf(
		entry(spec.KindSettings, "a", ".agnostic-ai/settings/a.yaml", map[string]any{"name": "a"}),
		entry(spec.KindSettings, "b", ".agnostic-ai/settings/b.yaml", map[string]any{"name": "b"}),
		pack,
	)
	if got == nil || got.Path != ".agnostic-ai/settings/defaults.yaml" || !slices.Contains(got.Paths, got.Path) {
		t.Fatalf("pack value edit = %+v", got)
	}
	got = editOf(entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/base.yaml", map[string]any{"name": "defaults", "permissions": map[string]any{"deny": []any{"Bash"}}}))
	if got == nil || got.Path != "" || !strings.Contains(got.DisabledReason, ".agnostic-ai/settings/base.yaml") {
		t.Fatalf("taken defaults edit = %+v", got)
	}
}

func localOverlay(shared spec.Entry, path string, own map[string]any) spec.Entry {
	merged := shared
	merged.Path, merged.Layer = path, "project-user"
	merged.Meta = map[string]any{}
	for k, v := range shared.Meta {
		merged.Meta[k] = v
	}
	for k, v := range own {
		merged.Meta[k] = v
	}
	base := shared
	merged.Base, merged.LocalMeta = &base, own
	return merged
}

func TestSettingsFields_LocalOverlayShowsItsValueBesideTheTeamValue(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	shared := entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/defaults.yaml", map[string]any{"name": "defaults", "model": "sonnet", "effort": "high"})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/defaults.yaml", map[string]any{"name": "defaults", "model": "opus"})
	byKey := map[string]SettingField{}
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{overlay}}}), SectionSettings).Fields {
		byKey[f.Key] = f
	}
	model := byKey["model"]
	if len(model.Set) != 2 {
		t.Fatalf("model set = %+v, want the local and the team value", model.Set)
	}
	if got := model.Set[0]; !got.Local || got.Path != ".agnostic-ai/local/settings/defaults.yaml" || got.Values[0].Value != "opus" {
		t.Errorf("local source = %+v", got)
	}
	if got := model.Set[1]; got.Local || got.Path != ".agnostic-ai/settings/defaults.yaml" || got.Values[0].Value != "sonnet" {
		t.Errorf("team source = %+v", got)
	}
	if got := model.Editable; got == nil || got.Path != ".agnostic-ai/settings/defaults.yaml" || got.LocalPath != ".agnostic-ai/local/settings/defaults.yaml" {
		t.Errorf("model edit = %+v", got)
	}
	effort := byKey["effort"]
	if len(effort.Set) != 1 || effort.Set[0].Local || effort.Set[0].Path != ".agnostic-ai/settings/defaults.yaml" {
		t.Errorf("inherited effort set = %+v, want the team file", effort.Set)
	}
}

func TestSettingsFields_LocalPathNamesTheOverlayOfTheEditedSpec(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	team := entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"name": "team", "model": "sonnet"})
	for _, tc := range []struct {
		name     string
		settings []spec.Entry
		global   bool
		want     string
	}{
		{"shared spec", []spec.Entry{team}, false, ".agnostic-ai/local/settings/team.yaml"},
		{"no spec", nil, false, ".agnostic-ai/local/settings/defaults.yaml"},
		{"global", nil, true, "local/settings/defaults.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Global: tc.global, Bundle: spec.Bundle{Settings: tc.settings}}), SectionSettings).Fields {
				if f.Key == "model" && (f.Editable == nil || f.Editable.LocalPath != tc.want) {
					t.Errorf("model edit = %+v, want local path %s", f.Editable, tc.want)
				}
			}
		})
	}
}

func TestBuiltinItems_MarkTheChoicesTheLocalConfigChanges(t *testing.T) {
	cfg := &config.Config{Builtins: []string{"memory", "caveman"}}
	in := Inputs{Cfg: cfg, BuiltinsTeam: []string{"memory", "handoff"}, BuiltinsLocal: true}
	local := map[string]bool{}
	for _, item := range sectionByID(t, Build(in), SectionSettings).Items {
		if item.Kind == kindBuiltin {
			local[item.Name] = item.Local
		}
	}
	if !local["handoff"] || !local["caveman"] || local["memory"] || local["rtk"] {
		t.Errorf("local built-ins = %v, want handoff and caveman", local)
	}
	in.BuiltinsLocal = false
	for _, item := range sectionByID(t, Build(in), SectionSettings).Items {
		if item.Local {
			t.Errorf("%s is local without a local builtins list", item.Name)
		}
	}
}

func TestSettingsFields_LocalPathFollowsTheSpecNameNotItsFileName(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	base := entry(spec.KindSettings, "base", ".agnostic-ai/settings/defaults.yaml", map[string]any{"name": "base", "model": "sonnet"})
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{base}}}), SectionSettings).Fields {
		if f.Key == "model" && (f.Editable == nil || f.Editable.LocalPath != ".agnostic-ai/local/settings/base.yaml") {
			t.Errorf("model edit = %+v", f.Editable)
		}
	}
}

func TestSettingsFields_LocalOverlayThatClearsAFieldShowsAsPersonal(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	shared := entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/defaults.yaml", map[string]any{"name": "defaults", "model": "sonnet"})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/defaults.yaml", map[string]any{"name": "defaults", "model": nil})
	delete(overlay.Meta, "model")
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{overlay}}}), SectionSettings).Fields {
		if f.Key != "model" {
			continue
		}
		if len(f.Set) != 2 || !f.Set[0].Local || f.Set[0].Values[0].Value != "null" || f.Set[1].Values[0].Value != "sonnet" {
			t.Errorf("model set = %+v, want the personal null beside the team value", f.Set)
		}
	}
}

func TestAuthorityFields_LocalOverlayMarksANestedPermissionAsPersonal(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	shared := entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/defaults.yaml", map[string]any{"name": "defaults", "permissions": map[string]any{"allow": []any{"Bash(ls)"}}})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/defaults.yaml", map[string]any{"name": "defaults", "permissions": map[string]any{"allow": []any{"Bash(go test)"}}})
	if !LocalSets(overlay, "permissions.allow") || LocalSets(overlay, "permissions.deny") {
		t.Errorf("LocalSets misreads the nested key")
	}
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{overlay}}}), SectionAuthority).Fields {
		if f.Key == "permissions.allow" && (len(f.Set) == 0 || !f.Set[0].Local) {
			t.Errorf("allow set = %+v, want a personal source", f.Set)
		}
	}
}

func TestBuiltinKind_MarksAPersonalListEvenWhenItMatchesTheTeamList(t *testing.T) {
	in := Inputs{Cfg: &config.Config{Builtins: []string{"memory"}}, BuiltinsTeam: []string{"memory"}, BuiltinsLocal: true}
	for _, local := range []bool{true, false} {
		in.BuiltinsLocal = local
		for _, k := range sectionByID(t, Build(in), SectionSettings).Kinds {
			if k.Kind == kindBuiltin && k.Local != local {
				t.Errorf("builtin kind local = %v, want %v", k.Local, local)
			}
		}
	}
}

func TestSettingsItem_ListsTheKeysItsOverlaySets(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	shared := entry(spec.KindSettings, "native", ".agnostic-ai/settings/native.yaml", map[string]any{"name": "native", "x-claude": map[string]any{"verbose": false, "theme": "dark"}})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/native.yaml", map[string]any{"name": "native", "x-claude": map[string]any{"verbose": true}})
	for _, item := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{overlay}}}), SectionSettings).Items {
		if item.Name == "native" && !slices.Equal(item.LocalKeys, []string{"x-claude.verbose"}) {
			t.Errorf("local keys = %v", item.LocalKeys)
		}
	}
}

func TestSettingsItem_ListsTheKeysOfAPersonalOnlySpec(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	mine := entry(spec.KindSettings, "mine", ".agnostic-ai/local/settings/mine.yaml", map[string]any{"name": "mine", "x-claude": map[string]any{"verbose": true}})
	mine.Layer = "project-user"
	for _, item := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{mine}}}), SectionSettings).Items {
		if item.Name == "mine" && !slices.Equal(item.LocalKeys, []string{"x-claude.verbose"}) {
			t.Errorf("local keys = %v", item.LocalKeys)
		}
	}
}

func TestSettingsFields_PerToolMapsAreEditable(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	in := Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{
		entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"name": "team", "model": map[string]any{"default": "sonnet", "codex": "gpt-5"}, "effort": map[string]any{"claude": "high"}, "x-claude": map[string]any{"model": "opus"}}),
	}}}
	byKey := map[string]SettingField{}
	for _, f := range sectionByID(t, Build(in), SectionSettings).Fields {
		byKey[f.Key] = f
	}
	for _, key := range []string{"model", "effort"} {
		if got := byKey[key].Editable; got == nil || got.DisabledReason != "" || got.Path != ".agnostic-ai/settings/team.yaml" {
			t.Errorf("%s edit = %+v", key, got)
		}
	}
	if got := byKey["x-"].Editable; got != nil {
		t.Errorf("native keys edit = %+v, want none", got)
	}
	effort := byKey["effort"].Editable
	if !slices.Equal(effort.ToolAllowed["claude"], []string{"high", "low", "medium", "xhigh"}) {
		t.Errorf("claude levels = %v", effort.ToolAllowed)
	}
	if levels, takes := effort.ToolAllowed["codex"]; !takes || levels == nil || len(levels) != 0 {
		t.Errorf("codex levels = %v, want any value", effort.ToolAllowed)
	}
}

func TestSettingsFields_ModelTierAndNestedValuesStayReadOnly(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}, Models: map[string]config.ModelTier{"fast": {Models: map[string]string{"claude": "haiku"}}}}
	tiered := entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"name": "team", "model": "fast"})
	b := spec.Bundle{Settings: []spec.Entry{tiered}}
	b.ApplyModelTiers(cfg.Models)
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: b}), SectionSettings).Fields {
		if f.Key == "model" && (f.Editable == nil || !strings.Contains(f.Editable.DisabledReason, "tier")) {
			t.Errorf("tier model edit = %+v", f.Editable)
		}
	}
	nested := entry(spec.KindSettings, "team", ".agnostic-ai/settings/team.yaml", map[string]any{"name": "team", "effort": map[string]any{"claude": map[string]any{"level": "high"}}})
	for _, f := range sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{nested}}}), SectionSettings).Fields {
		if f.Key == "effort" && (f.Editable == nil || f.Editable.DisabledReason == "") {
			t.Errorf("nested effort edit = %+v", f.Editable)
		}
	}
}
