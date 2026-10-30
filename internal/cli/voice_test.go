package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

const voiceHomeSpec = "voice:\n  enabled: true\n  mode: tap\n  language: es\n"

const claudeVoiceLanguageNote = "Claude reads the dictation language from its response language; set x-claude.language to change both"

func TestVoice_GlobalWritesAndRestoresOwnedKeys(t *testing.T) {
	for _, tc := range []struct{ name, original string }{
		{"absent", ""},
		{"other keys", "{\n  \"theme\": \"dark\"\n}\n"},
		{"different mode", "{\n\n  \"theme\": \"dark\", \"voice\": { \"mode\":  \"hold\" }\n}\n"},
		{"empty voice object", "{\"voice\": { }, \"theme\": \"dark\"}"},
		{"matching values", "{\"voice\":{\"enabled\":true,\"mode\":\"tap\"}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			if tc.original != "" {
				mustWriteGlobalTest(t, path, tc.original)
			}
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, voiceHomeSpec)
			_, warnings, err := runGlobalAgentTest("--only", "claude", "--backup")
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			if got := strings.Count(warnings, claudeVoiceLanguageNote); got != 1 {
				t.Errorf("language note printed %d times, want once:\n%s", got, warnings)
			}
			got := readGlobalTest(t, path)
			values, err := settingsValues(path, "json", []byte(got))
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]any{"voice.enabled": true, "voice.mode": "tap"} {
				if have, present := settingsLookup(values, key, "json"); !present || !sameSetting(have, want) {
					t.Errorf("%s = %v, want %v", key, have, want)
				}
			}
			if _, present := settingsLookup(values, "language", "json"); present {
				t.Errorf("voice.language reached Claude's response language: %s", got)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check after sync: %v\n%s", err, warnings)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("removal: %v\n%s", err, warnings)
			}
			if tc.original == "" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("created settings file remains: %v", err)
				}
			} else if got := readGlobalTest(t, path); got != tc.original {
				t.Errorf("removal changed original bytes:\n%q\nwant:\n%q", got, tc.original)
			}
		})
	}
}

func TestVoice_ConflictNamesTheNativeOverride(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"voice\":{\"mode\":\"hold\"}}\n"
	mustWriteGlobalTest(t, path, original)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), voiceHomeSpec)
	_, _, err := runGlobalAgentTest("--only", "claude")
	if err == nil || !strings.Contains(err.Error(), `set x-claude.voice.mode to "hold"`) {
		t.Errorf("conflict does not name the x-claude override: %v", err)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("conflict changed settings: %s", got)
	}
}

func TestVoice_LintsModeValues(t *testing.T) {
	for _, tc := range []struct {
		spec  string
		valid bool
	}{
		{voiceHomeSpec, true},
		{"voice:\n  mode: hold\n", true},
		{"voice: {}\n", true},
		{"voice:\n  mode: toggle\n", false},
		{"voice:\n  enabled: \"yes\"\n", false},
		{"voice:\n  language: 3\n", false},
		{"voice:\n  autoSubmit: true\n", false},
		{"voice: true\n", false},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), tc.spec)
			out, err := runRoot(t, "lint", "--global")
			if strings.Contains(out, "LINT054") == tc.valid {
				t.Errorf("LINT054 valid=%t: %v\n%s", tc.valid, err, out)
			}
			if _, err := runRoot(t, "validate", "--global"); (err == nil) != tc.valid {
				t.Errorf("validate valid=%t: %v", tc.valid, err)
			}
			_, warnings, err := runGlobalAgentTest("--only", "claude")
			if (err == nil) != tc.valid {
				t.Errorf("sync valid=%t: %v\n%s", tc.valid, err, warnings)
			}
			if !tc.valid {
				if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
					t.Errorf("sync wrote settings for an invalid voice spec: %v", err)
				}
			}
		})
	}
}

func TestVoice_ClaudeOverrideWins(t *testing.T) {
	for _, tc := range []struct {
		name, override string
		want           map[string]any
		absent         []string
	}{
		{"leaf", "x-claude:\n  voice:\n    mode: hold\n", map[string]any{"voice.enabled": true, "voice.mode": "hold"}, nil},
		{"whole object", "x-claude:\n  voice: {}\n", map[string]any{"voice": map[string]any{}}, []string{"voice.enabled", "voice.mode"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), voiceHomeSpec+tc.override)
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.want {
				if have, present := settingsLookup(values, key, "json"); !present || !sameSetting(have, want) {
					t.Errorf("%s = %v, want %v", key, have, want)
				}
			}
			for _, key := range tc.absent {
				if have, present := settingsLookup(values, key, "json"); present {
					t.Errorf("portable %s = %v survived the x-claude override", key, have)
				}
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check after sync: %v\n%s", err, warnings)
			}
			state, err := loadGlobalState(filepath.Join(source, "state", "global.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range tc.absent {
				if _, owned := state.Settings["claude"][key]; owned {
					t.Errorf("state owns overridden portable key %s", key)
				}
			}
		})
	}
}

func TestVoice_ExplainsMappingsAndUnsupportedTargets(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), voiceHomeSpec)
	out, err := runRoot(t, "explain", "--global", "settings/voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"voice.enabled", "voice.mode"} {
		if !strings.Contains(out, `key "`+key+`"`) {
			t.Errorf("missing Claude mapping %s: %s", key, out)
		}
	}
	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil || !strings.Contains(warnings, "voice") || !strings.Contains(warnings, "no voice dictation setting") {
		t.Errorf("missing unsupported-target note: %v\n%s", err, warnings)
	}
}

func TestVoice_ProjectSyncLeavesNativeVoiceAlone(t *testing.T) {
	globalAgentTestHome(t)
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(".agnostic-ai", "settings", "voice.yaml"), voiceHomeSpec)
	var notes bytes.Buffer
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	captureLogOut(t)
	if _, err := runRoot(t, "sync"); err != nil {
		t.Fatal(err)
	}
	if out := notes.String(); !strings.Contains(out, "voice is personal; only sync --global writes it") {
		t.Errorf("missing personal-scope note: %s", out)
	}
	data, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "voice") {
		t.Errorf("project sync wrote voice: %s", data)
	}
}

func TestVoice_StateVersionIsRefusedByThePreviousRelease(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(home, ".claude", "settings.json"), "{\"voice\":{\"mode\":\"hold\"}}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), voiceHomeSpec)
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	path := filepath.Join(source, "state", "global.json")
	state, err := loadGlobalState(path)
	if err != nil {
		t.Fatal(err)
	}
	if original, present := state.SettingsOriginals["claude"]["voice.mode"]; !present || original.Raw != `"hold"` {
		t.Errorf("state lacks the original voice.mode: %+v", state.SettingsOriginals)
	}
	if _, err := loadGlobalStateUpTo(path, 9); err == nil {
		t.Errorf("a release that reads state version 9 accepted this state file")
	}
}

func TestVoice_MigratesVersion9RetentionOriginals(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, path, "{\"cleanupPeriodDays\": 3650}\n")
	mustWriteGlobalTest(t, filepath.Join(source, "state", "global.json"), `{"version":9,"files":[],"settings":{"claude":{"cleanupPeriodDays":3650}},"settingsPaths":{"claude":"`+filepath.ToSlash(path)+`"},"sessionRetention":{"claude":{"key":"cleanupPeriodDays","raw":"30","present":true}}}`)
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != "{\"cleanupPeriodDays\": 30}\n" {
		t.Errorf("version 9 retention original not restored: %q", got)
	}
}

func TestVoice_ParentOverrideAfterPortableSyncRestoresOriginal(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"voice\":{\"mode\":  \"hold\"}}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, voiceHomeSpec)
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("initial sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, settings, voiceHomeSpec+"x-claude:\n  voice: {}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("override sync: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := values["voice"].(map[string]any); !ok || len(got) != 0 {
		t.Errorf("parent override = %#v, want empty object", values["voice"])
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after override: %v\n%s", err, warnings)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("remove override: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("removal = %q, want %q", got, original)
	}
}

func TestVoice_ParentOverrideCanReturnToPortableMembers(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"voice\":{\"mode\":  \"hold\"}}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "voice.yaml")
	for _, contents := range []string{voiceHomeSpec, voiceHomeSpec + "x-claude:\n  voice: {}\n", voiceHomeSpec} {
		mustWriteGlobalTest(t, settings, contents)
		if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
			t.Fatalf("sync: %v\n%s", err, warnings)
		}
		if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
			t.Fatalf("check: %v\n%s", err, warnings)
		}
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("removal = %q, want %q", got, original)
	}
}

func TestVoice_RemovalPreservesUserChanges(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, voiceHomeSpec)
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, path, "{\"voice\":{\"enabled\":false,\"mode\":\"tap\",\"autoSubmit\":true}}\n")
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	values, err := settingsValues(path, "json", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"voice.enabled", "voice.autoSubmit"} {
		if _, present := settingsLookup(values, key, "json"); !present {
			t.Errorf("user value %s removed", key)
		}
	}
	if _, present := settingsLookup(values, "voice.mode", "json"); present {
		t.Error("owned mode remains")
	}
}

func TestVoice_InitialParentOverrideRestoresOriginal(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".claude", "settings.json")
	original := "{\"voice\":{\"mode\":  \"hold\"}}\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, voiceHomeSpec+"x-claude:\n  voice: {}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatalf("sync override: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check: %v\n%s", err, warnings)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("removal: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("removal = %q, want %q", got, original)
	}
}

func TestVoice_EmptyOverrideThenPortableRestoresOriginal(t *testing.T) {
	for _, tc := range []struct{ name, original string }{
		{"absent", ""},
		{"other settings", "{\n  \"theme\": \"dark\"\n}\n"},
		{"existing empty voice", "{\"voice\": { }, \"theme\": \"dark\"}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".claude", "settings.json")
			if tc.original != "" {
				mustWriteGlobalTest(t, path, tc.original)
			}
			settings := filepath.Join(source, "settings", "voice.yaml")
			for _, contents := range []string{"x-claude:\n  voice: {}\n", voiceHomeSpec} {
				mustWriteGlobalTest(t, settings, contents)
				if _, warnings, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("removal: %v\n%s", err, warnings)
			}
			if tc.original == "" {
				if got, err := os.ReadFile(path); !os.IsNotExist(err) {
					t.Errorf("created settings file remains: %q, %v", got, err)
				}
			} else if got := readGlobalTest(t, path); got != tc.original {
				t.Errorf("removal = %q, want %q", got, tc.original)
			}
		})
	}
}
