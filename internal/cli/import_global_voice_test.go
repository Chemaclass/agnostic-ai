package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check after legacy import: %v\n%s", err, warnings)
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

func TestImportGlobal_ReadsVoiceAcrossNativeTools(t *testing.T) {
	home, source := globalAgentTestHome(t)
	files := map[string]string{
		".claude/settings.json": `{"voice":{"enabled":true,"mode":"tap"},"language":"Spanish"}` + "\n",
		".qwen/settings.json":   `{"general":{"voice":{"enabled":true,"mode":"tap","language":"es"}}}` + "\n",
		".gemini/settings.json": `{"experimental":{"voiceMode":true,"voice":{"activationMode":"toggle"}}}` + "\n",
		".grok/config.toml":     "[ui]\nvoice_capture_mode = 'toggle'\nvoice_stt_language = 'es'\n",
		".vibe/config.toml":     "voice_mode_enabled = true\n",
	}
	for file, contents := range files {
		mustWriteGlobalTest(t, filepath.Join(home, filepath.FromSlash(file)), contents)
	}
	if _, warnings, err := runImportGlobalTest("claude", "qwen", "gemini", "grok", "vibe"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got, want := readImportedVoice(t, source), (map[string]any{"enabled": true, "mode": "tap", "language": "es"}); !reflect.DeepEqual(got, want) {
		t.Errorf("voice = %#v, want %#v", got, want)
	}
	for file, contents := range files {
		if got := readGlobalTest(t, filepath.Join(home, filepath.FromSlash(file))); got != contents {
			t.Errorf("import changed %s", file)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude,qwen,gemini,grok,vibe", "--check"); err != nil {
		t.Errorf("check after import: %v\n%s", err, warnings)
	}
}

func TestImportGlobal_VoiceConflictsFollowCommandOrder(t *testing.T) {
	for _, first := range []string{"claude", "qwen"} {
		t.Run(first, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
			mustWriteGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"), `{"general":{"voice":{"enabled":false,"mode":"hold","language":"es"}}}`+"\n")
			second := "qwen"
			want := map[string]any{"enabled": true, "mode": "tap", "language": "es"}
			if first == "qwen" {
				second = "claude"
				want["enabled"], want["mode"] = false, "hold"
			}
			_, warnings, err := runImportGlobalTest(first, second)
			if err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readImportedVoice(t, source); !reflect.DeepEqual(got, want) {
				t.Errorf("voice = %#v, want %#v", got, want)
			}
			for _, member := range []string{"enabled", "mode"} {
				if !strings.Contains(warnings, second+": skipped voice."+member) || !strings.Contains(warnings, first+" already set it") {
					t.Errorf("missing conflict warning for %s:\n%s", member, warnings)
				}
			}
		})
	}
}

func TestImportGlobal_VoiceReversesHoldModes(t *testing.T) {
	for _, tc := range []struct{ target, file, native string }{
		{"gemini", ".gemini/settings.json", `{"experimental":{"voice":{"activationMode":"push-to-talk"}}}`},
		{"grok", ".grok/config.toml", "[ui]\nvoice_capture_mode = 'hold'\n"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(home, filepath.FromSlash(tc.file)), tc.native+"\n")
			if _, warnings, err := runImportGlobalTest(tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readImportedVoice(t, source)["mode"]; got != "hold" {
				t.Errorf("mode = %v, want hold", got)
			}
		})
	}
}

func TestImportGlobal_VoiceIgnoresInvalidNativeMembers(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":"yes","mode":["tap"]},"language":"Spanish"}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(source, globalImportSettingsSpec)); !os.IsNotExist(err) {
		t.Errorf("invalid native voice was imported: %v", err)
	}
}

func TestImportGlobal_LegacyVoiceCanChangeAndRestore(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{ \"voiceEnabled\":  true }\n"
	mustWriteGlobalTest(t, path, original)
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync legacy value: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("matching legacy value changed: %q", got)
	}
	settings := filepath.Join(source, globalImportSettingsSpec)
	mustWriteGlobalTest(t, settings, "target: claude\nvoice:\n  enabled: false\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("change enabled: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if value, present := settingsLookup(values, "voice.enabled", "json"); !present || value != false {
		t.Errorf("new canonical value = %v, present %t", value, present)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("remove canonical value: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("restored legacy file = %q, want %q", got, original)
	}
}

func TestImportGlobal_LegacyVoiceWithModeRestoresCreatedParent(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"voiceEnabled\":true}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\n  mode: hold\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync mode: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, present := settingsLookup(values, "voice.enabled", "json"); present {
		t.Error("matching legacy value gained a canonical key")
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check with mode: %v\n%s", err, warnings)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("restored legacy file = %q, want %q", got, original)
	}
}

func TestImportGlobal_LegacyVoiceDoesNotHideRestoredCanonicalValue(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"voiceEnabled\":true,\"voice\":{\"enabled\":false}}\n")
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\n  mode: tap\nx-claude:\n  voice: {}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("sync parent: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\n  mode: tap\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("replace parent: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if value, present := settingsLookup(values, "voice.enabled", "json"); !present || value != true {
		t.Errorf("portable enabled = %v, present %t", value, present)
	}
}

func TestImportGlobal_LegacyVoiceKeepsExplicitNativeOverride(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"voiceEnabled\":true}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), "voice:\n  enabled: true\nx-claude:\n  voice:\n    enabled: false\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("native override: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if value, present := settingsLookup(values, "voice.enabled", "json"); !present || value != false {
		t.Errorf("explicit native enabled = %v, present %t", value, present)
	}
}
