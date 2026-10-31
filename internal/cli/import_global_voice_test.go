package cli

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync uncovered tool: %v\n%s", err, warnings)
	}
	path := filepath.Join(home, ".qwen", "settings.json")
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if mode, present := settingsLookup(values, "general.voice.mode", "json"); !present || mode != "tap" {
		t.Errorf("uncovered Qwen mode = %v, present %t, want tap", mode, present)
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

func TestImportGlobal_ImportedVoiceIsSharedBeyondSourceTools(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readGlobalTest(t, filepath.Join(source, globalImportSettingsSpec))), &doc); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"target", "targets", "target-exclude", "targets-exclude"} {
		if value, present := doc[selector]; present {
			t.Errorf("imported shared voice has %s = %v", selector, value)
		}
	}
	path := filepath.Join(home, ".qwen", "settings.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("import wrote another tool's native settings: %v", err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync another tool: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"general.voice.enabled": true, "general.voice.mode": "tap"} {
		if got, present := settingsLookup(values, key, "json"); !present || got != want {
			t.Errorf("%s = %v, present %t, want %v", key, got, present, want)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen", "--check"); err != nil {
		t.Errorf("check another tool: %v\n%s", err, warnings)
	}
}

func TestImportGlobal_VoiceSkipsUnknownNativeModes(t *testing.T) {
	for _, tc := range []struct{ name, target, file, native string }{
		{"Gemini tap", "gemini", ".gemini/settings.json", `{"experimental":{"voice":{"activationMode":"tap"}}}`},
		{"Gemini hold", "gemini", ".gemini/settings.json", `{"experimental":{"voice":{"activationMode":"hold"}}}`},
		{"Grok tap", "grok", ".grok/config.toml", "[ui]\nvoice_capture_mode = 'tap'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(home, filepath.FromSlash(tc.file)), tc.native)
			if _, warnings, err := runImportGlobalTest(tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if _, err := os.Stat(filepath.Join(source, globalImportSettingsSpec)); !os.IsNotExist(err) {
				t.Errorf("unknown native mode was imported: %v", err)
			}
		})
	}
}

func TestImportGlobal_VoiceModelCompanionKeepsExistingFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "aaa.yaml"), "target: gemini\nvoice:\n  mode: hold\n")
	path := filepath.Join(source, "settings", "imported-models.yaml")
	original := "# personal models\nmodel:\n  codex: gpt-test\n"
	mustWriteGlobalTest(t, path, original)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".gemini", "settings.json"), `{"model":{"name":"gemini-pro"}}`+"\n")
	_, warnings, err := runImportGlobalTest("claude", "gemini")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("existing companion changed: %q", got)
	}
	if !strings.Contains(warnings, "skipped "+path+": it exists with other content") {
		t.Errorf("missing companion collision warning:\n%s", warnings)
	}
}

func TestImportGlobal_AddsVoiceToPriorModelImport(t *testing.T) {
	for _, original := range []string{
		"# preferences\nmodel:\n  claude: 'sonnet' # model\neffort:\n  claude: high # effort\n",
		"# preferences\r\nmodel:\r\n  claude: 'sonnet' # model\r\neffort:\r\n  claude: high # effort\r\n",
		"model: {claude: 'sonnet'}\neffort: {claude: high} # keep",
	} {
		t.Run(original, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(source, globalImportSettingsSpec)
			mustWriteGlobalTest(t, path, original)
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			nativePath := filepath.Join(home, ".claude", "settings.json")
			native := "{\"model\":\"sonnet\",\"effortLevel\":\"high\",\"voice\":{\"enabled\":true,\"mode\":\"tap\"}}\n"
			mustWriteGlobalTest(t, nativePath, native)
			if _, warnings, err := runImportGlobalTest("claude", "--dry-run"); err != nil {
				t.Fatalf("dry-run: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("dry-run changed prior settings: %q", got)
			}
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got, want := readImportedVoice(t, source), (map[string]any{"enabled": true, "mode": "tap"}); !reflect.DeepEqual(got, want) {
				t.Errorf("voice = %#v, want %#v", got, want)
			}
			if got := readGlobalTest(t, path); !strings.HasPrefix(got, original) {
				t.Errorf("existing settings bytes changed:\n%q", got)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("private mode changed: %v", info.Mode())
			}
			if got := readGlobalTest(t, nativePath); got != native {
				t.Errorf("native file changed: %q", got)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check after import: %v\n%s", err, warnings)
			}
		})
	}
}

func TestImportGlobal_PriorSettingsScopeDoesNotNarrowNewVoice(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(source, globalImportSettingsSpec)
	original := "# keep scope\ntarget: codex\nmodel: {codex: 'gpt-test'} # model\n"
	mustWriteGlobalTest(t, path, original)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("existing scoped settings changed: %q", got)
	}
	voicePath := filepath.Join(source, "settings", "imported-voice.yaml")
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(readGlobalTest(t, voicePath)), &doc); err != nil {
		t.Fatal(err)
	}
	if voice, present := doc[voiceField]; !present || !sameSetting(voice, map[string]any{"enabled": true, "mode": "tap"}) {
		t.Errorf("shared voice = %v, present %t", voice, present)
	}
	for _, selector := range []string{"target", "targets", "target-exclude", "targets-exclude"} {
		if value, present := doc[selector]; present {
			t.Errorf("new voice has %s = %v", selector, value)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync uncovered tool: %v\n%s", err, warnings)
	}
	qwen := readGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"))
	if !strings.Contains(qwen, `"tap"`) {
		t.Errorf("shared voice missing from Qwen: %s", qwen)
	}
}

func TestImportGlobal_PriorVoiceImportHandlesUnusualYAML(t *testing.T) {
	for _, tc := range []struct {
		name, original string
		separate       bool
	}{
		{"flow", "{model: {claude: 'sonnet'}} # root flow\n", false},
		{"anchor", "&models\nmodel: {claude: 'sonnet'} # anchored\n", true},
		{"document end", "---\nmodel: {claude: 'sonnet'}\n... # keep document end\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(source, globalImportSettingsSpec)
			mustWriteGlobalTest(t, path, tc.original)
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"sonnet","voice":{"enabled":true,"mode":"tap"}}`+"\n")
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			got := readGlobalTest(t, path)
			voicePath := path
			if tc.separate {
				if got != tc.original {
					t.Errorf("unsupported YAML changed: %q", got)
				}
				voicePath = filepath.Join(source, "settings", "imported-voice.yaml")
			} else if !strings.Contains(got, "model: {claude: 'sonnet'}") {
				t.Errorf("existing model bytes changed: %q", got)
			}
			var doc map[string]any
			if err := yaml.Unmarshal([]byte(readGlobalTest(t, voicePath)), &doc); err != nil {
				t.Fatal(err)
			}
			if voice, present := doc[voiceField]; !present || !sameSetting(voice, map[string]any{"enabled": true, "mode": "tap"}) {
				t.Errorf("voice = %v, present %t", voice, present)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check: %v\n%s", err, warnings)
			}
		})
	}
}

type importVoiceMutationWriter func([]byte) (int, error)

func (write importVoiceMutationWriter) Write(data []byte) (int, error) { return write(data) }

func TestImportGlobal_PriorVoiceImportStopsIfDestinationChanges(t *testing.T) {
	for _, change := range []string{"content", "identical replacement", "removal"} {
		t.Run(change, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(source, globalImportSettingsSpec)
			original := "model: {claude: 'sonnet'}\n"
			current := original
			mustWriteGlobalTest(t, path, original)
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"sonnet","voice":{"enabled":true}}`+"\n")
			mustWriteGlobalTest(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"docs":{"command":"docs-mcp"}}}`+"\n")
			mutated := false
			cmd := NewRootCmd("test")
			cmd.SetErr(io.Discard)
			cmd.SetOut(io.Discard)
			prefix := "wrote "
			if change == "content" {
				prefix = "warning: "
				mustWriteGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"), `{"general":{"voice":{"enabled":false}}}`+"\n")
			}
			writer := importVoiceMutationWriter(func(data []byte) (int, error) {
				if !mutated && strings.HasPrefix(string(data), prefix) {
					mutated = true
					switch change {
					case "content":
						current = "# changed by another writer\n" + original
						mustWriteGlobalTest(t, path, current)
					case "identical replacement":
						replacement := path + ".replacement"
						mustWriteGlobalTest(t, replacement, original)
						if err := os.Rename(replacement, path); err != nil {
							t.Fatal(err)
						}
					case "removal":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
				}
				return len(data), nil
			})
			if change == "content" {
				cmd.SetErr(writer)
			} else {
				cmd.SetOut(writer)
			}
			cmd.SetArgs([]string{"import", "--global", "claude", "qwen"})
			err := cmd.Execute()
			if !mutated {
				t.Fatal("destination was not changed during import")
			}
			if err == nil || !strings.Contains(err.Error(), "changed since it was read") {
				t.Errorf("error = %v, want destination change refusal", err)
			}
			if change == "removal" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("removed destination was recreated: %v", err)
				}
			} else if got := readGlobalTest(t, path); got != current {
				t.Errorf("concurrent content overwritten: %q, want %q", got, current)
			}
		})
	}
}

func TestImportGlobal_LayeredDestinationDoesNotNarrowOrDeleteVoice(t *testing.T) {
	for _, tc := range []struct{ name, shared, localImported, localFallback string }{
		{"local selector", "model: {codex: gpt-test}\n", "target: codex\n", ""},
		{"local null", "model: {codex: gpt-test}\n", "voice: null\n", ""},
		{"local only selector", "", "target: codex\n", ""},
		{"fallback selector", "target: codex\nmodel: {codex: gpt-test}\n", "", "target: codex\n"},
		{"fallback null", "target: codex\nmodel: {codex: gpt-test}\n", "", "voice: null\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			shared := filepath.Join(source, "settings", "imported.yaml")
			if tc.shared != "" {
				mustWriteGlobalTest(t, shared, tc.shared)
			}
			if tc.localImported != "" {
				mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "imported.yaml"), tc.localImported)
			}
			if tc.localFallback != "" {
				mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "imported-voice.yaml"), tc.localFallback)
				if tc.localFallback == "voice: null\n" {
					mustWriteGlobalTest(t, filepath.Join(source, "settings", "imported-voice.yaml"), "model: {codex: gpt-test}\n")
				}
			}
			nativePath := filepath.Join(home, ".claude", "settings.json")
			native := "{\"voice\":{\"enabled\":true,\"mode\":\"tap\"}}\n"
			mustWriteGlobalTest(t, nativePath, native)
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			path := filepath.Join(home, ".qwen", "settings.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Qwen settings missing: %v", err)
			}
			values, err := settingsValues(path, "json", data)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]any{"general.voice.enabled": true, "general.voice.mode": "tap"} {
				if got, ok := settingsLookup(values, key, "json"); !ok || !sameSetting(got, want) {
					t.Errorf("%s = %v, present %t, want %v", key, got, ok, want)
				}
			}
			if tc.shared != "" && readGlobalTest(t, shared) != tc.shared {
				t.Errorf("existing shared settings changed")
			}
			if tc.localImported != "" && readGlobalTest(t, filepath.Join(source, "local", "settings", "imported.yaml")) != tc.localImported {
				t.Error("local settings changed")
			}
			if tc.localFallback != "" && readGlobalTest(t, filepath.Join(source, "local", "settings", "imported-voice.yaml")) != tc.localFallback {
				t.Error("local fallback changed")
			}
			if tc.localFallback == "voice: null\n" && readGlobalTest(t, filepath.Join(source, "settings", "imported-voice.yaml")) != "model: {codex: gpt-test}\n" {
				t.Error("shared fallback changed")
			}
			if got := readGlobalTest(t, nativePath); got != native {
				t.Errorf("native changed: %q", got)
			}
		})
	}
}

func TestImportGlobal_NamedDestinationUsesEffectiveLocalOverlay(t *testing.T) {
	for _, overlay := range []string{"target: codex\n", "voice: null\n"} {
		t.Run(overlay, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(source, "settings", "imported.yaml")
			original := "name: personal\nmodel: {codex: gpt-test}\n"
			mustWriteGlobalTest(t, path, original)
			local := filepath.Join(source, "local", "settings", "personal.yaml")
			mustWriteGlobalTest(t, local, overlay)
			collision := filepath.Join(source, "local", "settings", "different.yaml")
			mustWriteGlobalTest(t, collision, "name: imported-voice\ntarget: codex\n")
			mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("shared settings changed: %q", got)
			}
			if got := readGlobalTest(t, local); got != overlay {
				t.Errorf("local settings changed: %q", got)
			}
			if _, err := os.Stat(filepath.Join(source, "settings", "imported-voice-2.yaml")); err != nil {
				t.Errorf("named fallback collision was not avoided: %v", err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			qwen := readGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"))
			if !strings.Contains(qwen, `"tap"`) {
				t.Errorf("shared voice missing from Qwen: %s", qwen)
			}
		})
	}
}

func TestImportGlobal_CompatibleLocalSettingsStillAllowVoiceAppend(t *testing.T) {
	home, source := globalAgentTestHome(t)
	shared := filepath.Join(source, "settings", "imported.yaml")
	mustWriteGlobalTest(t, shared, "model: {codex: gpt-test}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "settings", "imported.yaml"), "targets: []\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, shared); !strings.Contains(got, "voice:") {
		t.Errorf("compatible settings did not receive voice: %s", got)
	}
}

func TestImportGlobal_ShadowedDestinationDoesNotLoseVoice(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(source, "settings", "imported.yaml")
	original := "name: personal\nmodel: {codex: gpt-test}\n"
	mustWriteGlobalTest(t, path, original)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "later.yaml"), "name: personal\ntarget: codex\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("shadowed settings changed: %q", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".qwen", "settings.json")); err != nil {
		t.Errorf("Qwen voice missing: %v", err)
	}
}

func TestImportGlobal_NewDestinationDoesNotShadowNamedSettings(t *testing.T) {
	for _, filename := range []string{"a.yaml", "z.yaml"} {
		t.Run(filename, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(source, "settings", filename)
			original := "name: imported\ntarget: codex\nmodel: {codex: gpt-test}\n"
			mustWriteGlobalTest(t, path, original)
			native := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, native, `{"voice":{"enabled":true,"mode":"tap"}}`+"\n")
			if _, warnings, err := runImportGlobalTest("claude"); err != nil {
				t.Fatalf("import: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("existing settings changed: %q", got)
			}
			if _, err := os.Stat(filepath.Join(source, "settings", "imported.yaml")); !os.IsNotExist(err) {
				t.Errorf("created colliding settings: %v", err)
			}
			bundle, err := loadGlobalBundle(source, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := adapters.SettingsModel(bundle.Settings, "codex"); got != "gpt-test" {
				t.Errorf("existing model = %q", got)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			qwen := readGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"))
			if !strings.Contains(qwen, `"tap"`) {
				t.Errorf("Qwen voice missing: %s", qwen)
			}
		})
	}
}

func TestImportGlobal_NewDestinationExtendsCompatibleLocalOnlySettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	local := filepath.Join(source, "local", "settings", "imported.yaml")
	mustWriteGlobalTest(t, local, "targets: []\n")
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), `{"voice":{"enabled":true}}`+"\n")
	if _, warnings, err := runImportGlobalTest("claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, filepath.Join(source, "settings", "imported.yaml")); !strings.Contains(got, "voice:") {
		t.Errorf("voice not imported: %s", got)
	}
	if got := readGlobalTest(t, local); got != "targets: []\n" {
		t.Errorf("local settings changed: %q", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".qwen", "settings.json")); err != nil {
		t.Errorf("Qwen voice missing: %v", err)
	}
}
