package overview

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSettingsFields_VoiceEditContractFollowsScope(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	for _, global := range []bool{false, true} {
		fields := map[string]SettingField{}
		for _, field := range buildFields(SectionSettings, nil, nil, cfg, "", global) {
			fields[field.Key] = field
		}
		for _, tc := range []struct {
			key, label, kind string
			allowed          []string
		}{
			{"voice.enabled", "Voice dictation", "boolean", nil},
			{"voice.mode", "Voice mode", "text", []string{"hold", "tap"}},
			{"voice.language", "Voice language", "text", nil},
		} {
			field, present := fields[tc.key]
			if !present || field.Editable == nil {
				t.Errorf("global=%t: missing editable metadata for %s", global, tc.key)
				continue
			}
			edit := field.Editable
			if field.Label != tc.label || edit.Kind != tc.kind || !reflect.DeepEqual(edit.Allowed, tc.allowed) {
				t.Errorf("global=%t: %s = %+v, edit=%+v", global, tc.key, field, edit)
			}
			if global {
				if edit.Path != "settings/defaults.yaml" || edit.DisabledReason != "" || edit.SyncCommand != "agnostic-ai sync --global" {
					t.Errorf("global %s edit = %+v", tc.key, edit)
				}
			} else if edit.DisabledReason != "Voice is written only by sync --global. Open Settings in agnostic-ai ui --global." {
				t.Errorf("project %s disabled reason = %q", tc.key, edit.DisabledReason)
			}
		}
	}
}

func TestSettingsFields_VoiceValuesStayEditableAndSeparate(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	e := entry(spec.KindSettings, "voice", "settings/voice.yaml", map[string]any{
		"name": "voice", "voice": map[string]any{"enabled": false, "mode": "tap", "language": "es"},
	})
	fields := map[string]SettingField{}
	for _, field := range buildFields(SectionSettings, []SectionItem{sectionItem("", e)}, []spec.Entry{e}, cfg, "", true) {
		fields[field.Key] = field
	}
	for _, tc := range []struct{ key, kind, value string }{
		{"voice.enabled", ValueBool, "false"},
		{"voice.mode", ValueText, "tap"},
		{"voice.language", ValueText, "es"},
	} {
		field := fields[tc.key]
		if field.Editable == nil || field.Editable.DisabledReason != "" || field.Editable.Path != "settings/voice.yaml" {
			t.Errorf("%s edit = %+v", tc.key, field.Editable)
		}
		if len(field.Set) != 1 || len(field.Set[0].Values) != 1 {
			t.Errorf("%s sources = %+v", tc.key, field.Set)
			continue
		}
		if got := field.Set[0].Values[0]; got.Key != tc.key || got.Type != tc.kind || got.Value != tc.value {
			t.Errorf("%s value = %+v", tc.key, got)
		}
	}
}

func TestSettingsFields_ProjectVoiceValuesShowTheGlobalOnlyReason(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	e := entry(spec.KindSettings, "voice", ".agnostic-ai/settings/voice.yaml", map[string]any{
		"name": "voice", "voice": map[string]any{"enabled": false, "mode": "tap", "language": "es"},
	})
	fields := map[string]SettingField{}
	for _, field := range buildFields(SectionSettings, []SectionItem{sectionItem("", e)}, []spec.Entry{e}, cfg, "", false) {
		fields[field.Key] = field
	}
	for _, key := range []string{"voice.enabled", "voice.mode", "voice.language"} {
		field := fields[key]
		if len(field.Set) != 1 || len(field.Set[0].Values) != 1 {
			t.Errorf("%s does not show its configured value: %+v", key, field.Set)
		}
		if field.Editable == nil || field.Editable.DisabledReason != "Voice is written only by sync --global. Open Settings in agnostic-ai ui --global." {
			t.Errorf("%s project edit = %+v", key, field.Editable)
		}
	}
}

func TestSettingsValuePath_VoiceNamesOnlyTheOverriddenMember(t *testing.T) {
	shared := entry(spec.KindSettings, "voice", "settings/voice.yaml", map[string]any{
		"name": "voice", "voice": map[string]any{"enabled": true, "mode": "tap"},
	})
	personal := localOverlay(shared, "local/settings/voice.yaml", map[string]any{
		"name": "voice", "voice": map[string]any{"enabled": false, "mode": "tap"},
	})
	personal.LocalMeta["voice"] = map[string]any{"enabled": false}
	if got := SettingsValuePath("", personal, "voice.enabled", []string{"claude"}, nil); got != personal.Path {
		t.Errorf("enabled source = %q, want %q", got, personal.Path)
	}
	if got := SettingsValuePath("", personal, "voice.mode", []string{"claude"}, nil); got != shared.Path {
		t.Errorf("mode source = %q, want %q", got, shared.Path)
	}
}
