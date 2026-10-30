package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestImportGlobal_ReadsClaudeVoice(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	native := "{\n  \"voice\": {\"enabled\": true, \"mode\": \"tap\"},\n  \"language\": \"Spanish\"\n}\n"
	mustWriteGlobalTest(t, path, native)
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	voice := readImportedVoice(t, source)
	if want := (map[string]any{"enabled": true, "mode": "tap"}); !reflect.DeepEqual(voice, want) {
		t.Errorf("voice = %#v, want %#v; response language must stay out", voice, want)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("import changed native settings:\n%s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after import: %v\n%s", err, warnings)
	}
}

func TestImportGlobal_ClaudeVoiceEnabledFallback(t *testing.T) {
	for _, tc := range []struct {
		name, native string
		want         map[string]any
	}{
		{"legacy enabled", `{"voiceEnabled":true}`, map[string]any{"enabled": true}},
		{"legacy disabled", `{"voiceEnabled":false}`, map[string]any{"enabled": false}},
		{"nested wins", `{"voiceEnabled":true,"voice":{"enabled":false}}`, map[string]any{"enabled": false}},
		{"object blocks fallback", `{"voiceEnabled":true,"voice":{"mode":"hold"}}`, map[string]any{"mode": "hold"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), tc.native+"\n")
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readImportedVoice(t, source); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("voice = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestImportGlobal_VoiceSkipsOnlyTheCoveredTool(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		skip        bool
	}{
		{"all tools", "", true},
		{"Claude only", "target: claude\n", true},
		{"another tool", "target: codex\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			existing := tc.scope + "voice:\n  mode: hold\n"
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, existing)
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, settings); got != existing {
				t.Errorf("import changed existing home spec:\n%s", got)
			}
			if tc.skip {
				if _, err := os.Stat(filepath.Join(source, globalImportSettingsSpec)); !os.IsNotExist(err) {
					t.Errorf("covered voice was imported: %v", err)
				}
				return
			}
			if got, want := readImportedVoice(t, source), (map[string]any{"enabled": true, "mode": "tap"}); !reflect.DeepEqual(got, want) {
				t.Errorf("voice = %#v, want %#v", got, want)
			}
		})
	}
}

func readImportedVoice(t *testing.T, source string) map[string]any {
	t.Helper()
	data := readGlobalTest(t, filepath.Join(source, globalImportSettingsSpec))
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatalf("parse imported settings: %v", err)
	}
	voice, ok := doc["voice"].(map[string]any)
	if !ok {
		t.Fatalf("imported settings have no voice map:\n%s", data)
	}
	return voice
}

func TestImportGlobal_VoiceScopePreservesCoveredToolsAndTheirModels(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "aaa.yaml"), "target: gemini\nvoice:\n  mode: hold\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".gemini", "settings.json"), `{"model":{"name":"gemini-pro"}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude", "gemini"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	names, err := loadGlobalBuiltins(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	layers, err := globalLayers(source, names)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := spec.LoadLayered(layers)
	if err != nil {
		t.Fatal(err)
	}
	mode := ""
	for _, entry := range bundle.Settings {
		if value, ok := entry.Meta["voice"].(map[string]any); ok && entry.EmitsTo("gemini") {
			if own, ok := value["mode"].(string); ok {
				mode = own
			}
		}
	}
	if mode != "hold" {
		t.Errorf("covered Gemini voice mode = %q, want hold", mode)
	}
	if model := adapters.SettingsModel(bundle.Settings, "gemini"); model != "gemini-pro" {
		t.Errorf("Gemini model = %q, want gemini-pro", model)
	}
}
