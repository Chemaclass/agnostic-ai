package overview

import (
	"encoding/json"
	"path/filepath"
	"reflect"
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

func TestAuthorityFields_OnlyPermissionListsExposeListEditing(t *testing.T) {
	cfg := &config.Config{Sources: config.Sources{Settings: "config/policies"}}
	fields := buildFields(SectionAuthority, nil, nil, cfg, "", false)
	for _, field := range fields {
		if !PermissionListField(field.Key) {
			if field.Editable != nil {
				t.Errorf("%s unexpectedly editable: %+v", field.Key, field.Editable)
			}
			continue
		}
		if edit := field.Editable; edit == nil || edit.Kind != "permission-list" || edit.Path != "config/policies/permissions.yaml" || edit.LocalPath != ".agnostic-ai/local/settings/permissions.yaml" || edit.SyncCommand != "agnostic-ai sync" {
			t.Errorf("%s metadata = %+v", field.Key, edit)
		} else if len(edit.Sources) != 1 || edit.Sources[0] != (FieldEditSource{Path: edit.Path, Spec: "permissions", LocalPath: edit.LocalPath}) {
			t.Errorf("%s scaffold source = %+v", field.Key, edit.Sources)
		}
	}
}

func TestPermissionFieldEdit_RequiresAChoiceForSeveralSourcesAndRefusesPacks(t *testing.T) {
	a := entry(spec.KindSettings, "a", ".agnostic-ai/settings/a.yaml", map[string]any{"name": "a"})
	b := entry(spec.KindSettings, "b", ".agnostic-ai/settings/b.yaml", map[string]any{"name": "b"})
	pack := entry(spec.KindSettings, "pack", ".agnostic-ai/packs/team/settings/pack.yaml", map[string]any{"name": "pack"})
	pack.Layer = "pack:team"
	inactive := entry(spec.KindSettings, "inactive", ".agnostic-ai/settings/inactive.yaml", map[string]any{"name": "inactive", "targets": []any{"codex"}})
	for _, tc := range []struct {
		name        string
		entries     []spec.Entry
		path, local string
		paths       int
		disabled    bool
	}{
		{"one source", []spec.Entry{a}, a.Path, ".agnostic-ai/local/settings/a.yaml", 1, false},
		{"several sources", []spec.Entry{a, b}, "", "", 2, false},
		{"pack source", []spec.Entry{pack}, "", "", 0, true},
		{"inactive source", []spec.Entry{inactive}, "", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edit := PermissionFieldEdit(tc.entries, &config.Config{Targets: []string{"claude"}}, "", false)
			if edit.Path != tc.path || edit.LocalPath != tc.local || len(edit.Paths) != tc.paths || (edit.DisabledReason != "") != tc.disabled {
				t.Errorf("metadata = %+v", edit)
			}
		})
	}
}

func TestAuthorityFields_PermissionChoicesNameTheirSpecsAndPersonalFiles(t *testing.T) {
	shared := entry(spec.KindSettings, "team-policy", ".agnostic-ai/settings/a.yaml", map[string]any{"name": "team-policy", "effort": "high"})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/mine.yaml", map[string]any{
		"name": "team-policy", "permissions": map[string]any{"ask": []any{"shell"}},
	})
	other := entry(spec.KindSettings, "other-policy", ".agnostic-ai/settings/b.yaml", map[string]any{"name": "other-policy", "effort": "low"})
	entries := []spec.Entry{overlay, other}
	fields := buildFields(SectionAuthority, []SectionItem{sectionItem("", overlay), sectionItem("", other)}, entries, &config.Config{Targets: []string{"claude"}}, "", false)
	at := slices.IndexFunc(fields, func(field SettingField) bool { return field.Key == "permissions.ask" })
	if at < 0 {
		t.Fatal("permissions.ask field is missing")
	}
	field := fields[at]
	if len(field.Set) != 1 || !field.Set[0].Local || field.Set[0].Spec != shared.Name {
		t.Fatalf("ask sources = %+v, want only the personal value", field.Set)
	}
	data, err := json.Marshal(field.Editable)
	if err != nil {
		t.Fatal(err)
	}
	var edit struct {
		Path    string `json:"path"`
		Sources []struct {
			Path      string `json:"path"`
			Spec      string `json:"spec"`
			LocalPath string `json:"localPath"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &edit); err != nil {
		t.Fatal(err)
	}
	if edit.Path != "" || len(edit.Sources) != 2 {
		t.Fatalf("permission metadata = %s, want two explicit source choices", data)
	}
	if got := edit.Sources[0]; got.Path != shared.Path || got.Spec != shared.Name || got.LocalPath != overlay.Path {
		t.Errorf("first edit choice = %+v", got)
	}
	if got := edit.Sources[1]; got.Path != other.Path || got.Spec != other.Name || got.LocalPath != ".agnostic-ai/local/settings/other-policy.yaml" {
		t.Errorf("second edit choice = %+v", got)
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

func TestSettingsFields_NativeValuesKeepTheirSharedAndPersonalSources(t *testing.T) {
	shared := entry(spec.KindSettings, "native", ".agnostic-ai/settings/native.yaml", map[string]any{
		"name": "native", "x-claude": map[string]any{"verbose": false, "theme": "dark"},
	})
	personalPath := ".agnostic-ai/local/settings/native.yaml"
	overlay := localOverlay(shared, personalPath, map[string]any{
		"name": "native", "x-claude": map[string]any{"verbose": true},
	})
	overlay.Meta["x-claude"] = map[string]any{"verbose": true, "theme": "dark"}
	cleared := localOverlay(shared, personalPath, map[string]any{
		"name": "native", "x-claude": map[string]any{"verbose": nil},
	})
	cleared.Meta["x-claude"] = map[string]any{"verbose": nil, "theme": "dark"}
	inherited := localOverlay(shared, personalPath, map[string]any{"name": "native", "effort": "high"})
	mine := entry(spec.KindSettings, "native", personalPath, map[string]any{
		"name": "native", "x-claude": map[string]any{"verbose": true},
	})
	mine.Layer = "project-user"
	sharedSource := FieldSource{Spec: "native", Path: shared.Path, Values: []Value{
		{Key: "x-claude.theme", Type: ValueText, Value: "dark"},
		{Key: "x-claude.verbose", Type: ValueBool, Value: "false"},
	}}
	personalSource := FieldSource{Spec: "native", Path: personalPath, Local: true, Values: []Value{
		{Key: "x-claude.verbose", Type: ValueBool, Value: "true"},
	}}
	clearedSource := FieldSource{Spec: "native", Path: personalPath, Local: true, Values: []Value{
		{Key: "x-claude.verbose", Type: ValueText, Value: "null"},
	}}
	for _, tc := range []struct {
		name string
		spec spec.Entry
		want []FieldSource
	}{
		{"overridden native key", overlay, []FieldSource{personalSource, sharedSource}},
		{"cleared native key", cleared, []FieldSource{clearedSource, sharedSource}},
		{"inherited native keys", inherited, []FieldSource{sharedSource}},
		{"personal-only spec", mine, []FieldSource{personalSource}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := buildFields(SectionSettings, []SectionItem{sectionItem("", tc.spec)}, []spec.Entry{tc.spec}, nil, "", false)
			at := slices.IndexFunc(fields, func(field SettingField) bool { return field.Key == "x-" })
			if at < 0 {
				t.Fatal("native field is missing")
			}
			if got := fields[at].Set; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("native sources = %+v, want %+v", got, tc.want)
			}
		})
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

func TestAuthorityFields_WholePersonalPermissionsClearKeepsSharedValues(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	shared := entry(spec.KindSettings, "policy", ".agnostic-ai/settings/policy.yaml", map[string]any{
		"permissions": map[string]any{"allow": []any{"read"}, "ask": []any{"shell"}, "deny": []any{"edit"}, "default-mode": "plan"},
	})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/policy.yaml", map[string]any{"permissions": nil})
	delete(overlay.Meta, "permissions")
	section := sectionByID(t, Build(Inputs{Cfg: cfg, Bundle: spec.Bundle{Settings: []spec.Entry{overlay}}}), SectionAuthority)
	if len(section.Items) != 1 || section.Items[0].Path != overlay.Path || counts(section)["Permissions"] != 1 {
		t.Errorf("Authority items = %+v, counts = %v, want the personal clear in Permissions", section.Items, counts(section))
	}
	for _, field := range section.Fields {
		if !strings.HasPrefix(field.Key, "permissions.") {
			continue
		}
		if len(field.Set) != 2 {
			t.Errorf("%s sources = %+v, want personal clear and shared baseline", field.Key, field.Set)
			continue
		}
		if own := field.Set[0]; !own.Local || own.Path != overlay.Path || len(own.Values) != 1 || own.Values[0].Key != "permissions" || own.Values[0].Value != "null" {
			t.Errorf("%s personal clear = %+v", field.Key, own)
		}
		if team := field.Set[1]; team.Local || team.Path != shared.Path || len(team.Values) != 1 || team.Values[0].Key != field.Key {
			t.Errorf("%s shared baseline = %+v", field.Key, team)
		}
	}
	other := entry(spec.KindSettings, "other", ".agnostic-ai/settings/other.yaml", map[string]any{"model": "sonnet"})
	edit := PermissionFieldEdit([]spec.Entry{overlay, other}, cfg, "", false)
	data, err := json.Marshal(edit.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"disabledReason":`) || !strings.Contains(string(data), "Remove permissions: null") || !strings.Contains(string(data), overlay.Path) {
		t.Errorf("permission choices = %s, want source-specific clear instructions", data)
	}
}

func TestSettingsFields_PersonalMapKeepsNestedNullAndSharedBaseline(t *testing.T) {
	shared := entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/defaults.yaml", map[string]any{
		"model": map[string]any{"default": "sonnet", "codex": "gpt-5"},
	})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/defaults.yaml", map[string]any{"model": map[string]any{"codex": nil}})
	overlay.Meta["model"] = map[string]any{"default": "sonnet"}
	fields := buildFields(SectionSettings, []SectionItem{sectionItem("", overlay)}, []spec.Entry{overlay}, &config.Config{Targets: []string{"claude", "codex"}}, "", false)
	at := slices.IndexFunc(fields, func(field SettingField) bool { return field.Key == "model" })
	if at < 0 {
		t.Fatal("model field is missing")
	}
	want := []FieldSource{
		{Spec: shared.Name, Path: overlay.Path, Local: true, Values: []Value{{Key: "model.codex", Type: ValueText, Value: "null"}}},
		{Spec: shared.Name, Path: shared.Path, Values: []Value{{Key: "model.codex", Type: ValueText, Value: "gpt-5"}, {Key: "model.default", Type: ValueText, Value: "sonnet"}}},
	}
	if got := fields[at].Set; !reflect.DeepEqual(got, want) {
		t.Errorf("model sources = %+v, want %+v", got, want)
	}
}

func TestSettingsFields_PersonalTierKeepsDerivedEffortAndItsEditGuard(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Models: map[string]config.ModelTier{
		"fast": {Models: map[string]string{"claude": "haiku"}, Effort: "low"},
	}}
	shared := entry(spec.KindSettings, "defaults", ".agnostic-ai/settings/defaults.yaml", map[string]any{"model": "sonnet"})
	overlay := localOverlay(shared, ".agnostic-ai/local/settings/defaults.yaml", map[string]any{"model": "fast"})
	bundle := spec.Bundle{Settings: []spec.Entry{overlay}}
	bundle.ApplyModelTiers(cfg.Models)
	fields := buildFields(SectionSettings, []SectionItem{sectionItem("", bundle.Settings[0])}, bundle.Settings, cfg, "", false)
	byKey := map[string]SettingField{}
	for _, field := range fields {
		byKey[field.Key] = field
	}
	model := byKey["model"]
	if len(model.Set) != 2 || len(model.Set[0].Values) != 1 || model.Set[0].Values[0].Key != "model" || model.Set[0].Values[0].Value != "fast" {
		t.Errorf("model sources = %+v, want the personal tier name and shared baseline", model.Set)
	}
	if model.Editable == nil || !strings.Contains(model.Editable.DisabledReason, "tier") {
		t.Errorf("model edit = %+v, want the tier edit guard", model.Editable)
	}
	effort := byKey["effort"]
	if len(effort.Set) != 1 || len(effort.Set[0].Values) != 1 || effort.Set[0].Values[0].Value != "low" {
		t.Errorf("effort sources = %+v, want the derived tier effort", effort.Set)
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

func TestSettingsOverride_CountsOnlyToolsTheLaterSpecGivesAValue(t *testing.T) {
	both := []string{"claude", "codex"}
	a := spec.Entry{Name: "a", Meta: map[string]any{"model": "gpt-5.4", "effort": "high"}}
	for _, tc := range []struct {
		name     string
		later    map[string]any
		claude   int
		codex    int
		inEffect int
	}{
		{"claude entry only", map[string]any{"model": map[string]any{"claude": "opus"}, "effort": map[string]any{"claude": "low"}}, 1, -1, 0},
		{"default", map[string]any{"model": map[string]any{"default": "gpt-5.5"}, "effort": map[string]any{"default": "low"}}, 1, 1, 1},
	} {
		settings := []spec.Entry{a, {Name: "b", Meta: tc.later}}
		for _, key := range []string{"model", "effort"} {
			if got := SettingsOverride(settings, 0, key, []string{"claude"}, false); got != tc.claude {
				t.Errorf("%s %s claude override = %d, want %d", tc.name, key, got, tc.claude)
			}
			if got := SettingsOverride(settings, 0, key, []string{"codex"}, false); got != tc.codex {
				t.Errorf("%s %s codex override = %d, want %d", tc.name, key, got, tc.codex)
			}
			if got := SettingsInEffect(settings, key, both, false); got != tc.inEffect {
				t.Errorf("%s %s in effect = %d, want %d", tc.name, key, got, tc.inEffect)
			}
		}
	}
}

func TestSettingsOverride_SkipsALaterClaudeModelForToolsThatDropIt(t *testing.T) {
	settings := []spec.Entry{
		{Name: "a", Meta: map[string]any{"model": map[string]any{"codex": "gpt-5.4"}}},
		{Name: "b", Meta: map[string]any{"model": "claude-opus-4"}},
	}
	if got := SettingsOverride(settings, 0, "model", []string{"codex"}, false); got != -1 {
		t.Errorf("codex override = %d, want -1", got)
	}
	if got := SettingsOverride(settings, 0, "model", []string{"claude"}, false); got != 1 {
		t.Errorf("claude override = %d, want 1", got)
	}
	if got := SettingsOverride(settings, 0, "model", []string{"codex"}, true); got != 1 {
		t.Errorf("global codex override = %d, want 1", got)
	}
}
