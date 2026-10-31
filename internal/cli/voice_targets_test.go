package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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
		for _, before := range []string{"absent", "empty", "unrelated", "existing values"} {
			t.Run(tc.target+"/"+before, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				path := filepath.Join(home, filepath.FromSlash(tc.file))
				original := tc.original
				switch before {
				case "absent":
					original = ""
				case "empty":
					original = ""
					if tc.format == "json" {
						original = "{}"
					}
				case "unrelated":
					original = "# My preferences\nnarrator_enabled = true\n"
					if tc.format == "json" {
						original = "{\n  \"theme\": \"dark\"\n}\n"
					} else if tc.target == "grok" {
						original = "# My preferences\n[ui]\ntheme = 'dark'\n"
					}
				}
				if before != "absent" {
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
				if before == "absent" {
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
		name, native, extra string
		wantNote            bool
	}{
		{"absent", "", "", true},
		{"empty", "{\"voiceModel\":\"\"}\n", "", true},
		{"configured", "{\"voiceModel\":\"whisper\"}\n", "", false},
		{"same run", "", "x-qwen:\n  voiceModel: whisper\n", false},
		{"same run empty", "{\"voiceModel\":\"whisper\"}\n", "x-qwen:\n  voiceModel: ''\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			if tc.native != "" {
				mustWriteGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"), tc.native)
			}
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec+tc.extra)
			_, warnings, err := runGlobalAgentTest("--only", "qwen", "--backup")
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
		{"warp", []string{"Warp has no voice capture mode setting"}},
		{"kiro", []string{"Kiro CLI has no dictation on/off setting", "Kiro CLI has no voice capture mode setting"}},
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

func TestVoiceTargets_EveryUnmappedTargetHasASpecificNote(t *testing.T) {
	for _, target := range adapters.Names() {
		if globalTargets[target].settings.voice != nil {
			continue
		}
		if reason := voiceTargetNoOpReason(target); reason == "" || reason == "no voice dictation setting sync --global can write" {
			t.Errorf("%s has no specific voice note", target)
		}
	}
}

func TestVoiceTargets_RemovalKeepsUserEdits(t *testing.T) {
	for _, tc := range []struct{ target, file, edited string }{
		{"qwen", ".qwen/settings.json", "{\"general\":{\"voice\":{\"enabled\":false,\"mode\":\"tap\",\"language\":\"es\"}}}\n"},
		{"gemini", ".gemini/settings.json", "{\"experimental\":{\"voiceMode\":false,\"voice\":{\"activationMode\":\"toggle\"}}}\n"},
		{"grok", ".grok/config.toml", "[ui]\nvoice_capture_mode='hold' # user choice\nvoice_stt_language='es'\n"},
		{"vibe", ".vibe/config.toml", "voice_mode_enabled = false # user choice\n"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, targetVoiceSpec)
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			mustWriteGlobalTest(t, path, tc.edited)
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			data := readGlobalTest(t, path)
			key, format := "general.voice.enabled", "json"
			switch tc.target {
			case "gemini":
				key = "experimental.voiceMode"
			case "grok":
				key, format = "ui.voice_capture_mode", grokTOMLFormat
			case "vibe":
				key, format = "voice_mode_enabled", "toml"
			}
			values, err := settingsValues(path, format, []byte(data))
			if err != nil {
				t.Fatal(err)
			}
			want := any(false)
			if tc.target == "grok" {
				want = "hold"
			}
			if got, found := settingsLookup(values, key, format); !found || !sameSetting(got, want) {
				t.Errorf("user value = %v, want %v", got, want)
			}
		})
	}
}

func TestVoiceTargets_RestoreTOMLValuePreservesUserFormatting(t *testing.T) {
	for _, tc := range []struct{ name, target, path, spec, original, edited, want string }{
		{"vibe changed comment", "vibe", ".vibe/config.toml", "voice:\n  enabled: true\n", "voice_mode_enabled = false # old\n", "voice_mode_enabled = true # new\n", "voice_mode_enabled = false # new\n"},
		{"vibe added comment", "vibe", ".vibe/config.toml", "voice:\n  enabled: true\n", "voice_mode_enabled = false\n", "voice_mode_enabled = true # new\n", "voice_mode_enabled = false # new\n"},
		{"vibe deleted comment", "vibe", ".vibe/config.toml", "voice:\n  enabled: true\n", "voice_mode_enabled = false # old\n", "voice_mode_enabled = true\n", "voice_mode_enabled = false\n"},
		{"grok changed comment", "grok", ".grok/config.toml", "voice:\n  mode: tap\n", "[ui]\nvoice_capture_mode = 'hold' # old\n", "[ui]\nvoice_capture_mode = \"toggle\" # new\n", "[ui]\nvoice_capture_mode = 'hold' # new\n"},
		{"grok spacing", "grok", ".grok/config.toml", "voice:\n  mode: tap\n", "[ui]\nvoice_capture_mode = 'hold'\n", "[ui]\n  voice_capture_mode  =   \"toggle\"  \n", "[ui]\n  voice_capture_mode  =   'hold'  \n"},
		{"grok multiline literal", "grok", ".grok/config.toml", "voice:\n  mode: tap\n", "[ui]\r\nvoice_capture_mode = '''hold''' # old\r\n", "[ui]\r\nvoice_capture_mode = \"toggle\" # new\r\n", "[ui]\r\nvoice_capture_mode = '''hold''' # new\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, filepath.FromSlash(tc.path))
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, path, tc.original)
			mustWriteGlobalTest(t, settings, tc.spec)
			if _, warnings, err := runGlobalAgentTest("--only", tc.target, "--backup"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			mustWriteGlobalTest(t, path, tc.edited)
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVoiceTargets_QwenNotesRemovedModel(t *testing.T) {
	const note = "Qwen keeps dictation off until a voice model is chosen"
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\nx-qwen:\n  voiceModel: whisper\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("initial sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\n")
	_, warnings, err := runGlobalAgentTest("--only", "qwen")
	if err != nil {
		t.Fatalf("second sync: %v\n%s", err, warnings)
	}
	if count := strings.Count(warnings, note); count != 1 {
		t.Errorf("note count = %d, want 1: %s", count, warnings)
	}
	data := readGlobalTest(t, filepath.Join(home, ".qwen", "settings.json"))
	if strings.Contains(data, "voiceModel") {
		t.Errorf("model remains: %s", data)
	}
}

func TestVoiceTargets_QwenKeepsUserChangedModelWithoutNote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\nx-qwen:\n  voiceModel: whisper\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("initial sync: %v\n%s", err, warnings)
	}
	path := filepath.Join(home, ".qwen", "settings.json")
	data := readGlobalTest(t, path)
	mustWriteGlobalTest(t, path, strings.Replace(data, "whisper", "custom", 1))
	mustWriteGlobalTest(t, settings, "voice:\n  enabled: true\n")
	_, warnings, err := runGlobalAgentTest("--only", "qwen")
	if err != nil {
		t.Fatalf("second sync: %v\n%s", err, warnings)
	}
	if strings.Contains(warnings, "Qwen keeps dictation off") {
		t.Errorf("unexpected note: %s", warnings)
	}
	if got := readGlobalTest(t, path); !strings.Contains(got, "custom") {
		t.Errorf("user model removed: %s", got)
	}
}

func TestGrokTOML_TableSectionsPreserveOffsets(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        []string
	}{
		{"empty", "", nil},
		{"BOM and CRLF", "\xef\xbb\xbf[ui]\r\nvoice_capture_mode = 'hold'\r\n[other]\r\nx = 1\r\n[ui.extra]\r\nx = 2", []string{"[ui]\r\nvoice_capture_mode = 'hold'\r\n", "[ui.extra]\r\nx = 2"}},
		{"final newline", "[other]\nx = 1\n[ui]\nx = 2\n", []string{"[ui]\nx = 2\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sections, err := grokTableSections([]byte(tc.input), "ui")
			if err != nil {
				t.Fatal(err)
			}
			if len(sections) != len(tc.want) {
				t.Fatalf("got %d sections, want %d", len(sections), len(tc.want))
			}
			for i, section := range sections {
				if got := tc.input[section.start:section.end]; got != tc.want[i] {
					t.Errorf("section %d = %q, want %q", i, got, tc.want[i])
				}
			}
		})
	}
}

func TestVoiceTargets_GrokParentRestorePreservesUserComments(t *testing.T) {
	for _, tc := range []struct{ name, original, edited, want string }{
		{"body", "[ui]\nvoice_capture_mode='hold'\n", "[ui]\n# new user note\n", "[ui]\n# new user note\nvoice_capture_mode='hold'\n"},
		{"header and separated sections", "[ui] # old\nvoice_capture_mode='hold'\n[plugins]\nx=1\n[ui.extra]\ny=2\n", "[plugins]\nx=1\n[ui] # new\n# note\n", "[ui] # new\n# note\nvoice_capture_mode='hold'\n[plugins]\nx=1\n[ui.extra]\ny=2\n"},
		{"adopted empty table", "[ui] # old\n# old body\n", "[ui] # new\n# new body\n", "[ui] # new\n# new body\n"},
		{"child first", "[ui.status_line]\nenabled=true\n[plugins]\nx=1\n[ui]\nvoice_capture_mode='hold'\n", "[ui]\n# note\n[plugins]\nx=1\n", "[ui.status_line]\nenabled=true\n[plugins]\nx=1\n[ui]\n# note\nvoice_capture_mode='hold'\n"},
		{"header without newline", "[ui]\nvoice_capture_mode='hold'\n", "[ui] # new", "[ui] # new\nvoice_capture_mode='hold'\n"},
		{"header without newline, CRLF original", "[ui]\r\nvoice_capture_mode='hold'\r\n", "[ui] # new", "[ui] # new\r\nvoice_capture_mode='hold'\r\n"},
		{"child and parent together", "[ui.status_line]\nenabled=true\n[ui]\nvoice_capture_mode='hold'\n", "[ui]\n# note\n", "[ui.status_line]\nenabled=true\n[ui]\n# note\nvoice_capture_mode='hold'\n"},
		{"child only", "[ui.status_line]\nenabled=true\n", "[ui]\n# note\n", "[ui]\n# note\n[ui.status_line]\nenabled=true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".grok", "config.toml")
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, path, tc.original)
			mustWriteGlobalTest(t, settings, "voice:\n  mode: tap\nx-grok:\n  ui: {}\n")
			if _, warnings, err := runGlobalAgentTest("--only", "grok", "--backup"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			mustWriteGlobalTest(t, path, tc.edited)
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVoiceTargets_GrokRestoresMultilineQuoteEndings(t *testing.T) {
	for _, tc := range []struct{ name, yaml, native string }{
		{"four basic quotes", `'es"'`, `"""es""""`},
		{"five basic quotes", `'es""'`, `"""es"""""`},
		{"four literal quotes", `"es'"`, `'''es''''`},
		{"five literal quotes", `"es''"`, `'''es'''''`},
		{"hash and escape", `'es#"de'`, `"""es#\"de"""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".grok", "config.toml")
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, path, "[ui]\r\nvoice_stt_language = 'en'\r\n")
			mustWriteGlobalTest(t, settings, "voice:\n  language: "+tc.yaml+"\n")
			if _, warnings, err := runGlobalAgentTest("--only", "grok", "--backup"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			mustWriteGlobalTest(t, path, "[ui]\r\nvoice_stt_language = "+tc.native+" # new\r\n")
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if got, want := readGlobalTest(t, path), "[ui]\r\nvoice_stt_language = 'en' # new\r\n"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
