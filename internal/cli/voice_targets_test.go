package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const targetVoiceSpec = "voice:\n  enabled: true\n  mode: tap\n  language: es\n"

func TestVoiceTargets_WriteCheckAndRestore(t *testing.T) {
	for _, tc := range []struct {
		target, file, format, original string
		want                           map[string]any
	}{
		{"qwen", ".qwen/settings.json", "json", "{\n  \"voiceModel\": \"whisper\", \"general\": {\"voice\": {\"enabled\": false, \"mode\": \"hold\", \"language\": \"english\"}}\n}\n", map[string]any{"general.voice.enabled": true, "general.voice.mode": "tap", "general.voice.language": "es"}},
		{"gemini", ".gemini/settings.json", "json", "{\n  \"experimental\": {\"voiceMode\": false, \"voice\": {\"activationMode\": \"push-to-talk\"}}, \"theme\": \"dark\"\n}\n", map[string]any{"experimental.voiceMode": true, "experimental.voice.activationMode": "toggle"}},
		{"grok", ".grok/config.toml", grokTOMLFormat, "# My preferences\n[ui]\nvoice_capture_mode = 'hold' # Keep this comment\nvoice_stt_language = 'en'\n", map[string]any{"ui.voice_capture_mode": "toggle", "ui.voice_stt_language": "es"}},
		{"vibe", ".vibe/config.toml", "toml", "# My preferences\nvoice_mode_enabled = false # Keep this comment\nnarrator_enabled = true\n", map[string]any{"voice_mode_enabled": true}},
	} {
		for _, before := range []string{"absent", "unrelated", "existing values"} {
			t.Run(tc.target+"/"+before, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, filepath.FromSlash(tc.file))
				original := tc.original
				switch before {
				case "absent":
					original = ""
				case "unrelated":
					original = "# My preferences\nnarrator_enabled = true\n"
					if tc.format == "json" {
						original = "{\n  \"theme\": \"dark\"\n}\n"
					} else if tc.target == "grok" {
						original = "# My preferences\n[ui]\ntheme = 'dark'\n"
					}
				}
				if original != "" {
					mustWriteGlobalTest(t, path, original)
				}
				specPath := filepath.Join(source, "settings", "voice.yaml")
				mustWriteGlobalTest(t, specPath, targetVoiceSpec)
				if _, warnings, err := runGlobalAgentTest("--only", tc.target, "--backup"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("native voice settings: %v", err)
				}
				values, err := settingsValues(path, tc.format, data)
				if err != nil {
					t.Fatal(err)
				}
				for key, want := range tc.want {
					if got, found := settingsLookup(values, key, tc.format); !found || !sameSetting(got, want) {
						t.Errorf("%s = %v (present %t), want %v", key, got, found, want)
					}
				}
				if tc.target == "grok" {
					if _, found := settingsLookup(values, "ui.voice_keybind_enabled", tc.format); found {
						t.Error("portable enabled changed the Grok shortcut")
					}
				}
				if _, warnings, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
					t.Errorf("check after sync: %v\n%s", err, warnings)
				}
				if err := os.Remove(specPath); err != nil {
					t.Fatal(err)
				}
				if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
					t.Fatalf("remove voice: %v\n%s", err, warnings)
				}
				if original == "" {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("created file remains: %v", err)
					}
				} else if got := readGlobalTest(t, path); got != original {
					t.Errorf("restored bytes = %q, want %q", got, original)
				}
			})
		}
	}
}

func TestVoiceTargets_TranslateHoldMode(t *testing.T) {
	for _, tc := range []struct{ target, file, format, key, want string }{
		{"qwen", ".qwen/settings.json", "json", "general.voice.mode", "hold"},
		{"gemini", ".gemini/settings.json", "json", "experimental.voice.activationMode", "push-to-talk"},
		{"grok", ".grok/config.toml", grokTOMLFormat, "ui.voice_capture_mode", "hold"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), "voice:\n  mode: hold\n")
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			values, err := settingsValues(path, tc.format, data)
			if err != nil {
				t.Fatal(err)
			}
			if got, found := settingsLookup(values, tc.key, tc.format); !found || got != tc.want {
				t.Errorf("%s = %v, want %s", tc.key, got, tc.want)
			}
		})
	}
}

func TestVoiceTargets_QwenNoteDependsOnVoiceModel(t *testing.T) {
	const note = "Qwen keeps dictation off until a voice model is chosen; run /model --voice or set x-qwen.voiceModel"
	for _, tc := range []struct {
		name, native string
		wantNote     bool
	}{
		{"absent", "", true},
		{"empty", "{\"voiceModel\":\"\"}\n", true},
		{"configured", "{\"voiceModel\":\"whisper\"}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			if tc.native != "" {
				mustWriteGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"), tc.native)
			}
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
			_, warnings, err := runGlobalAgentTest("--only", "qwen")
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			wantCount := 0
			if tc.wantNote {
				wantCount = 1
			}
			if got := strings.Count(warnings, note); got != wantCount {
				t.Errorf("voice model note count = %d, want %d:\n%s", got, wantCount, warnings)
			}
		})
	}
}

func TestVoiceTargets_GrokExplainsEnabledLimit(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
	_, warnings, err := runGlobalAgentTest("--only", "grok")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	const note = "Grok has no dictation on/off key; /voice always works. Set x-grok.ui.voice_keybind_enabled to turn the shortcut off"
	if got := strings.Count(warnings, note); got != 1 {
		t.Errorf("enabled note count = %d, want once:\n%s", got, warnings)
	}
}

func TestVoiceTargets_NativeParentOverridesPortableValues(t *testing.T) {
	for _, tc := range []struct {
		target, file, format, override string
		want                           map[string]any
		absent                         []string
	}{
		{"qwen", ".qwen/settings.json", "json", "x-qwen:\n  general:\n    voice: {}\n", map[string]any{"general.voice": map[string]any{}}, []string{"general.voice.enabled", "general.voice.mode", "general.voice.language"}},
		{"gemini", ".gemini/settings.json", "json", "x-gemini:\n  experimental: {}\n", map[string]any{"experimental": map[string]any{}}, []string{"experimental.voiceMode", "experimental.voice.activationMode"}},
		{"grok", ".grok/config.toml", grokTOMLFormat, "x-grok:\n  ui: {}\n", map[string]any{"ui": map[string]any{}}, []string{"ui.voice_capture_mode", "ui.voice_stt_language"}},
		{"vibe", ".vibe/config.toml", "toml", "x-vibe:\n  voice_mode_enabled: false\n", map[string]any{"voice_mode_enabled": false}, nil},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec+tc.override)
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			values, err := settingsValues(path, tc.format, data)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.want {
				if got, found := settingsLookup(values, key, tc.format); !found || !sameSetting(got, want) {
					t.Errorf("%s = %v (present %t), want %v", key, got, found, want)
				}
			}
			for _, key := range tc.absent {
				if got, found := settingsLookup(values, key, tc.format); found {
					t.Errorf("overridden %s remains: %v", key, got)
				}
			}
			if _, warnings, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Errorf("check after override: %v\n%s", err, warnings)
			}
		})
	}
}

func TestVoiceTargets_ExplainListsAllWrittenKeys(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
	out, err := runRoot(t, "explain", "--global", "settings/voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"voice.enabled", "voice.mode", "general.voice.enabled", "general.voice.mode", "general.voice.language",
		"experimental.voiceMode", "experimental.voice.activationMode", "ui.voice_capture_mode", "ui.voice_stt_language", "voice_mode_enabled",
	} {
		if !strings.Contains(out, `key "`+key+`"`) {
			t.Errorf("missing %s mapping:\n%s", key, out)
		}
	}
}

func TestVoiceTargets_ExplainUnsupportedMembersAndDeferredMappings(t *testing.T) {
	for _, tc := range []struct {
		target string
		notes  []string
	}{
		{"gemini", []string{"Gemini has no dictation language setting"}},
		{"vibe", []string{"Vibe has no voice capture mode setting", "Vibe has no dictation language setting"}},
		{"warp", []string{"#2836"}},
		{"kiro", []string{"#2837"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			_, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
			_, warnings, err := runGlobalAgentTest("--only", tc.target)
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			for _, note := range tc.notes {
				if got := strings.Count(warnings, note); got != 1 {
					t.Errorf("note %q count = %d, want once:\n%s", note, got, warnings)
				}
			}
		})
	}
}
