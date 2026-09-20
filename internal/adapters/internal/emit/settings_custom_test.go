package emit

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func settingsEntry(name string, meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: name, Meta: meta}
}

func TestSettingsCustomKeys_ReturnsTheTargetBlock(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{
			"model": "portable-model",
			"x-factory": map[string]any{
				"sandbox": map[string]any{"enabled": true},
			},
			"x-kilo": map[string]any{"sandbox": map[string]any{"enabled": false}},
		}),
	}
	got := SettingsCustomKeys(entries, "factory")
	want := map[string]any{"sandbox": map[string]any{"enabled": true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_LastSpecWinsPerKey(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"sandbox":     map[string]any{"enabled": true},
			"disableSpin": true,
		}}),
		settingsEntry("project", map[string]any{"x-factory": map[string]any{
			"sandbox": map[string]any{"enabled": false},
		}}),
	}
	got := SettingsCustomKeys(entries, "factory")
	want := map[string]any{
		"sandbox":     map[string]any{"enabled": false},
		"disableSpin": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_SkipsExcludedKeys(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-augment": map[string]any{
			"toolPermissions": []any{map[string]any{"toolName": "terminal"}},
			"shell":           "/bin/zsh",
		}}),
	}
	got := SettingsCustomKeys(entries, "augment", "toolPermissions")
	want := map[string]any{"shell": "/bin/zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_NilWithoutABlock(t *testing.T) {
	entries := []spec.Entry{settingsEntry("base", map[string]any{"model": "m"})}
	if got := SettingsCustomKeys(entries, "factory"); got != nil {
		t.Errorf("SettingsCustomKeys = %#v, want nil", got)
	}
}

func TestMergeSettingsCustomKeys_OverridesTheManagedKey(t *testing.T) {
	keys := map[string]any{"model": "portable-model"}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"model":   "droid-core",
			"sandbox": map[string]any{"enabled": true},
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "factory")
	if keys["model"] != "droid-core" {
		t.Errorf("model = %#v, want the x-factory block to win", keys["model"])
	}
	if _, ok := keys["sandbox"]; !ok {
		t.Errorf("sandbox missing: %#v", keys)
	}
}

func TestMergeSettingsCustomKeys_LeavesManagedKeysAloneWithoutABlock(t *testing.T) {
	keys := map[string]any{"model": "portable-model"}
	MergeSettingsCustomKeys(keys, []spec.Entry{settingsEntry("base", nil)}, "factory")
	if len(keys) != 1 || keys["model"] != "portable-model" {
		t.Errorf("keys = %#v, want the managed map untouched", keys)
	}
}
