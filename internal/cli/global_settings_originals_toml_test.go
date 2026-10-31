package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVoiceTargets_RestoreTOMLAssignmentsExactly(t *testing.T) {
	for _, tc := range []struct{ target, file, original string }{
		{"grok", ".grok/config.toml", "# preferences\n[ui]\n  voice_capture_mode   =   'hold' # mode\nvoice_stt_language=  'en' # language\n[plugins]\nenabled = true\n"},
		{"vibe", ".vibe/config.toml", "# preferences\n  voice_mode_enabled=false # voice\nnarrator_enabled = true\n"},
		{"vibe", ".vibe/config.toml", "voice_mode_enabled=false"},
		{"vibe", ".vibe/config.toml", "# keep\r\n  voice_mode_enabled=false # voice\r\nnarrator_enabled=true\r\n"},
		{"grok", ".grok/config.toml", "# keep\r\n[ui]\r\nvoice_capture_mode = 'hold' # mode\r\nvoice_stt_language='en'\r\n[plugins]\r\nx=1\r\n"},
		{"grok", ".grok/config.toml", " \t\r\n\r\n"},
		{"vibe", ".vibe/config.toml", " \t\r\n\r\n"},
	} {
		t.Run(tc.target+"/"+tc.original, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, filepath.FromSlash(tc.file))
			mustWriteGlobalTest(t, path, tc.original)
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, targetVoiceSpec)
			if _, warnings, err := runGlobalAgentTest("--only", tc.target, "--backup"); err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != tc.original {
				t.Errorf("restored %q, want %q", got, tc.original)
			}
		})
	}
}

func TestVoiceTargets_GrokParentOverrideRestoresOriginalTable(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".grok", "config.toml")
	original := "# preferences\n[ui]\nvoice_capture_mode = 'hold' # mode\nvoice_stt_language = 'en'\ntheme = 'dark'\n[plugins]\nenabled = true\n"
	mustWriteGlobalTest(t, path, original)
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, targetVoiceSpec)
	if _, warnings, err := runGlobalAgentTest("--only", "grok", "--backup"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, settings, targetVoiceSpec+"x-grok:\n  ui: {}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "grok", "--backup"); err != nil {
		t.Fatalf("parent override: %v\n%s", err, warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
		t.Errorf("check after override: %v\n%s", err, warnings)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("restored %q, want %q", got, original)
	}
}

func TestVoiceTargets_GrokParentOverrideRestoresSeparatedChildTables(t *testing.T) {
	for _, original := range []string{
		"[ui]\nvoice_capture_mode='hold'\n[plugins]\nenabled=true\n[ui.extra]\nkeep=true\n",
		"[ui.extra]\nkeep=true\n[plugins]\nenabled=true\n[ui]\nvoice_capture_mode='hold'",
		"[ui]\nvoice_capture_mode='hold'\n[[plugins]]\nname='first'\n[ui.extra]\nkeep=true\n[[plugins]]\nname='second'\n[ui.other]\nkeep=false\n",
	} {
		t.Run(original, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".grok", "config.toml")
			mustWriteGlobalTest(t, path, original)
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, targetVoiceSpec+"x-grok:\n  ui: {}\n")
			if _, warnings, err := runGlobalAgentTest("--only", "grok", "--backup"); err != nil {
				t.Fatalf("parent override: %v\n%s", err, warnings)
			}
			values, err := settingsValues(path, grokTOMLFormat, []byte(readGlobalTest(t, path)))
			if err != nil {
				t.Fatal(err)
			}
			if ui, found := settingsLookup(values, "ui", grokTOMLFormat); !found || !sameSetting(ui, map[string]any{}) {
				t.Errorf("native ui = %v, want empty table", ui)
			}
			originalValues, err := settingsValues(path, grokTOMLFormat, []byte(original))
			if err != nil {
				t.Fatal(err)
			}
			if !sameSetting(values["plugins"], originalValues["plugins"]) {
				t.Errorf("unrelated plugins = %v, want %v", values["plugins"], originalValues["plugins"])
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok", "--check"); err != nil {
				t.Errorf("check after override: %v\n%s", err, warnings)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "grok"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("restored %q, want %q", got, original)
			}
		})
	}
}
