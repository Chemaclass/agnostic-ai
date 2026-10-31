package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func warpVoiceTestPath(t *testing.T, home string) string {
	t.Helper()
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Local", "warp", "Warp", "config", "settings.toml")
	case "linux":
		return filepath.Join(home, ".config", "warp-terminal", "settings.toml")
	default:
		return filepath.Join(home, ".warp", "settings.toml")
	}
}

func TestWarpVoice_WriteCheckAndRestore(t *testing.T) {
	for _, tc := range []struct {
		name, original string
		absent         bool
	}{
		{name: "absent", absent: true},
		{name: "empty"},
		{name: "whitespace", original: " \r\n\t"},
		{name: "empty table", original: "[agents.voice]"},
		{name: "unrelated tables", original: "# preferences\n[agents]\nother=true\n[appearance]\nspacing='compact'"},
		{name: "unrelated", original: "# preferences\n[agents.voice]\nvoice_input_toggle_key = 'fn' # shortcut\n[appearance]\nspacing = 'compact'"},
		{name: "existing", original: "# preferences\r\n[agents . 'voice'] # dictation\r\nvoice_input_enabled = false # on/off\r\nvoice_input_language='en' # language\r\nvoice_input_toggle_key='fn' # shortcut\r\n[appearance]\r\nspacing='compact'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := warpVoiceTestPath(t, home)
			if !tc.absent {
				mustWriteGlobalTest(t, path, tc.original)
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			portable := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, portable, targetVoiceSpec)
			if _, notes, err := runGlobalAgentTest("--only", "warp", "--backup"); err != nil {
				t.Fatalf("sync: %v\n%s", err, notes)
			} else if !strings.Contains(notes, "Warp has no voice capture mode setting") {
				t.Errorf("missing mode note: %s", notes)
			}
			check := func(enabled bool) {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				values, err := settingsValues(path, "toml", data)
				if err != nil {
					t.Fatal(err)
				}
				for key, want := range map[string]any{"agents.voice.voice_input_enabled": enabled, "agents.voice.voice_input_language": "es"} {
					if got, ok := settingsLookup(values, key, "json"); !ok || !sameSetting(got, want) {
						t.Errorf("%s = %v, present %t, want %v", key, got, ok, want)
					}
				}
				if strings.Contains(tc.original, "voice_input_toggle_key") {
					if got, _ := settingsLookup(values, "agents.voice.voice_input_toggle_key", "json"); got != "fn" {
						t.Errorf("changed shortcut: %v", got)
					}
				}
				if _, notes, err := runGlobalAgentTest("--only", "warp", "--check"); err != nil {
					t.Errorf("check: %v\n%s", err, notes)
				}
			}
			check(true)
			mustWriteGlobalTest(t, portable, "voice:\n  enabled: false\n  language: es\n")
			if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
				t.Fatalf("update: %v\n%s", err, notes)
			}
			check(false)
			if err := os.Remove(portable); err != nil {
				t.Fatal(err)
			}
			if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
				t.Fatalf("remove: %v\n%s", err, notes)
			}
			if tc.absent {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("created file remains: %v", err)
				}
			} else {
				if got := readGlobalTest(t, path); got != tc.original {
					t.Errorf("restored bytes %q, want %q", got, tc.original)
				}
				if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Errorf("private mode not preserved: %v, %v", info, err)
				}
			}
		})
	}
}

func TestWarpVoice_ExplainUsesNativeSettingsPath(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := warpVoiceTestPath(t, home)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
	out, err := runRoot(t, "explain", "--global", "settings/voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{path, "agents.voice.voice_input_enabled", "agents.voice.voice_input_language"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain misses %q: %s", want, out)
		}
	}
}

func TestWarpVoice_UsesPlatformSettingsPaths(t *testing.T) {
	home := t.TempDir()
	xdg, local := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("LOCALAPPDATA", local)
	for _, tc := range []struct{ goos, want string }{
		{"darwin", filepath.Join(home, ".warp", "settings.toml")},
		{"linux", filepath.Join(xdg, "warp-terminal", "settings.toml")},
		{"windows", filepath.Join(local, "warp", "Warp", "config", "settings.toml")},
	} {
		if got := globalPath(home, globalTargets["warp"].settings.platformPath(tc.goos)); got != tc.want {
			t.Errorf("%s path %q, want %q", tc.goos, got, tc.want)
		}
	}
	t.Setenv("LOCALAPPDATA", "")
	if got, want := globalPath(home, globalTargets["warp"].settings.windowsPath), filepath.Join(home, "AppData", "Local", "warp", "Warp", "config", "settings.toml"); got != want {
		t.Errorf("Windows fallback %q, want %q", got, want)
	}
}

func TestWarpVoice_RejectsRelativePlatformRoot(t *testing.T) {
	for _, tc := range []struct{ goos, variable string }{
		{"linux", "XDG_CONFIG_HOME"},
		{"windows", "LOCALAPPDATA"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			t.Setenv(tc.variable, "relative")
			if err := globalTargets["warp"].settings.platformRootError(tc.goos); err == nil || !strings.Contains(err.Error(), tc.variable) {
				t.Errorf("relative platform root accepted: %v", err)
			}
		})
	}
}

func TestWarpVoice_LanguageMatchesNativeFormat(t *testing.T) {
	for _, language := range []string{"es", "", "en-US", "auto", "Spanish", "ES"} {
		t.Run(language, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := warpVoiceTestPath(t, home)
			original := "[agents.voice]\nvoice_input_language = 'en' # preference\n"
			mustWriteGlobalTest(t, path, original)
			portable := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, portable, "voice:\n  language: '"+language+"'\n")
			_, notes, err := runGlobalAgentTest("--only", "warp", "--backup")
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, notes)
			}
			if language != "es" && language != "" {
				if got := readGlobalTest(t, path); got != original || !strings.Contains(notes, "two-letter language code") {
					t.Errorf("unsupported language changed native bytes or missed note: %q\n%s", got, notes)
				}
			} else {
				values, err := settingsValues(path, "toml", []byte(readGlobalTest(t, path)))
				if err != nil {
					t.Fatal(err)
				}
				if got, _ := settingsLookup(values, "agents.voice.voice_input_language", "json"); got != language {
					t.Errorf("native language %v, want %q", got, language)
				}
			}
			if _, notes, err := runGlobalAgentTest("--only", "warp", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, notes)
			}
			if err := os.Remove(portable); err != nil {
				t.Fatal(err)
			}
			if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
				t.Fatalf("remove: %v\n%s", err, notes)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("restored language bytes %q, want %q", got, original)
			}
		})
	}
}

func TestWarpVoice_ModeOnlyDoesNotCreateSettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := warpVoiceTestPath(t, home)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), "voice:\n  mode: tap\n")
	if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil || !strings.Contains(notes, "Warp has no voice capture mode setting") {
		t.Fatalf("mode-only sync: %v\n%s", err, notes)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("unsupported mode created settings: %v", err)
	}
}

func TestWarpVoice_NativeOverrideKeepsShortcut(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := warpVoiceTestPath(t, home)
	original := "[agents.voice]\nvoice_input_enabled=true\nvoice_input_language='en'\nvoice_input_toggle_key='fn' # keep\n"
	mustWriteGlobalTest(t, path, original)
	portable := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, portable, targetVoiceSpec+"x-warp:\n  agents:\n    voice:\n      voice_input_enabled: false\n      voice_input_language: ''\n      voice_input_toggle_key: control_left\n")
	if _, notes, err := runGlobalAgentTest("--only", "warp", "--backup"); err != nil || !strings.Contains(notes, "keep other settings in Warp") {
		t.Fatalf("native override: %v\n%s", err, notes)
	}
	values, err := settingsValues(path, "toml", []byte(readGlobalTest(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"voice_input_enabled": false, "voice_input_language": "", "voice_input_toggle_key": "fn"} {
		if got, _ := settingsLookup(values, "agents.voice."+key, "json"); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if err := os.Remove(portable); err != nil {
		t.Fatal(err)
	}
	if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("remove: %v\n%s", err, notes)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("restored %q, want %q", got, original)
	}
}

func TestWarpVoice_RemovalKeepsUserChanges(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := warpVoiceTestPath(t, home)
	mustWriteGlobalTest(t, path, "[agents.voice]\nvoice_input_enabled=false\nvoice_input_language='en'\n")
	portable := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, portable, targetVoiceSpec)
	if _, notes, err := runGlobalAgentTest("--only", "warp", "--backup"); err != nil {
		t.Fatalf("sync: %v\n%s", err, notes)
	}
	edited := "[agents.voice]\nvoice_input_enabled=false # changed\nvoice_input_language='fr' # changed\nvoice_input_toggle_key='fn'\n"
	mustWriteGlobalTest(t, path, edited)
	if err := os.Remove(portable); err != nil {
		t.Fatal(err)
	}
	if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("remove: %v\n%s", err, notes)
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Errorf("user changes lost: %q", got)
	}
}

func TestWarpVoice_ExistingGlobalImportRoundTripsReadOnly(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := warpVoiceTestPath(t, home)
	original := "# preferences\n[agents.voice]\nvoice_input_enabled=false\nvoice_input_language='es'\nvoice_input_toggle_key='fn' # keep\n"
	mustWriteGlobalTest(t, path, original)
	if _, err := runRoot(t, "import", "--global", "warp"); err != nil {
		t.Fatal(err)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("import changed native bytes: %q", got)
	}
	imported := readGlobalTest(t, filepath.Join(source, "settings", "imported.yaml"))
	for _, want := range []string{"enabled: false", "language: es"} {
		if !strings.Contains(imported, want) {
			t.Errorf("import misses %s: %s", want, imported)
		}
	}
	if strings.Contains(imported, "toggle_key") {
		t.Errorf("import included shortcut: %s", imported)
	}
	if _, notes, err := runGlobalAgentTest("--only", "warp", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, notes)
	}
	if _, notes, err := runGlobalAgentTest("--only", "warp"); err != nil {
		t.Fatalf("sync: %v\n%s", err, notes)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("round trip changed native bytes: %q", got)
	}
}

func TestWarpVoice_UnsupportedTableShapesStayUnchanged(t *testing.T) {
	for _, original := range []string{
		"agents.voice = { voice_input_enabled = false, voice_input_language = 'en' }\n",
		"[agents]\nvoice = { voice_input_enabled = false, voice_input_language = 'en' }\n",
		"agents.voice.voice_input_enabled = false\nagents.voice.voice_input_language = 'en'\n",
	} {
		t.Run(original, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := warpVoiceTestPath(t, home)
			mustWriteGlobalTest(t, path, original)
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
			if _, _, err := runGlobalAgentTest("--only", "warp", "--backup"); err == nil || !strings.Contains(err.Error(), "rewrite it as [agents.voice]") {
				t.Errorf("unsupported shape error: %v", err)
			}
			if got := readGlobalTest(t, path); got != original {
				t.Errorf("refused edit changed native bytes: %q", got)
			}
			if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("refused edit changed private mode: %v, %v", info, err)
			}
		})
	}
}
