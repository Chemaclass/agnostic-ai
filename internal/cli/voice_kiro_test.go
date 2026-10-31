package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVoiceKiro_WriteCheckAndRestore(t *testing.T) {
	for _, original := range []string{"absent", "{}", `{ "voice.language" : "es", "voice.autoSubmit": false }`, "{\n  \"voice.autoSubmit\": false,\n  \"voice.language\" : \"en\",\n  \"chat.defaultModel\": \"custom\",\n  \"voice\": {\"language\": \"nested\"}\n}\n"} {
		t.Run(original, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			root := filepath.Join(home, "kiro-profile")
			t.Setenv("KIRO_HOME", root)
			path := filepath.Join(root, "settings", "cli.json")
			if original != "absent" {
				mustWriteGlobalTest(t, path, original)
			}
			settings := filepath.Join(source, "settings", "voice.yaml")
			mustWriteGlobalTest(t, settings, targetVoiceSpec)
			_, warnings, err := runGlobalAgentTest("--only", "kiro", "--backup")
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, warnings)
			}
			data := readGlobalTest(t, path)
			var values map[string]any
			if err := json.Unmarshal([]byte(data), &values); err != nil {
				t.Fatal(err)
			}
			if values["voice.language"] != "es" {
				t.Errorf("literal voice.language = %v; file %s", values["voice.language"], data)
			}
			if original != "absent" && original != "{}" {
				if want := strings.Replace(original, `"en"`, `"es"`, 1); data != want {
					t.Errorf("changed unowned bytes: got %q want %q", data, want)
				}
			} else if _, nested := values["voice"]; nested {
				t.Errorf("created nested voice: %s", data)
			}
			for _, note := range []string{"Kiro CLI has no dictation on/off setting", "Kiro CLI has no voice capture mode setting"} {
				if strings.Count(warnings, note) != 1 {
					t.Errorf("missing note %q: %s", note, warnings)
				}
			}
			if _, warnings, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, warnings)
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
				t.Fatalf("remove: %v\n%s", err, warnings)
			}
			if original == "absent" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("created file remains: %v", err)
				}
			} else if got := readGlobalTest(t, path); got != original {
				t.Errorf("restore = %q, want %q", got, original)
			}
		})
	}
}

func TestVoiceKiro_ExplainLanguage(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kiro-profile")
	t.Setenv("KIRO_HOME", root)
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "voice.yaml"), targetVoiceSpec)
	out, err := runRoot(t, "explain", "--global", "settings/voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.ToSlash(filepath.Join(root, "settings", "cli.json"))) || !strings.Contains(out, `key "voice.language"`) {
		t.Errorf("missing Kiro language mapping: %s", out)
	}
}

func TestVoiceKiro_NativeOverrideAndRemovalKeepUserChoice(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(source, "settings", "voice.yaml")
	path := filepath.Join(home, ".kiro", "settings", "cli.json")
	original := `{ "voice.language": "en", "voice.autoSubmit": false }`
	mustWriteGlobalTest(t, path, original)
	mustWriteGlobalTest(t, settings, "voice:\n  language: es\nx-kiro:\n  voice.language: auto\n  voice: {language: nested}\n  hooks: {unsafe: true}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro", "--backup"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	data := readGlobalTest(t, path)
	var values map[string]any
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		t.Fatal(err)
	}
	if nested, ok := values["voice"].(map[string]any); !ok || nested["language"] != "nested" {
		t.Errorf("literal and nested keys conflict: %s", data)
	}
	if values["voice.language"] != "auto" || values["voice.autoSubmit"] != false {
		t.Errorf("native override: %s", data)
	}
	if values["hooks"] != nil {
		t.Errorf("reserved hooks written: %s", data)
	}
	mustWriteGlobalTest(t, path, strings.Replace(data, `"auto"`, `"de"`, 1))
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, path); got != strings.Replace(original, `"en"`, `"de"`, 1) {
		t.Errorf("user choice lost: %s", got)
	}
}

func TestVoiceKiro_ImportReadsOnlyLanguage(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".kiro", "settings", "cli.json")
	native := `{ "voice.language": "auto", "voice.autoSubmit": false, "chat.defaultModel": "custom", "voice": {"language": "nested"} }`
	mustWriteGlobalTest(t, path, native)
	if _, warnings, err := runImportGlobalTest("kiro"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	voice := readImportedVoice(t, source)
	if len(voice) != 1 || voice["language"] != "auto" {
		t.Errorf("imported voice = %#v", voice)
	}
	if got := readGlobalTest(t, path); got != native {
		t.Errorf("import changed native bytes: %s", got)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, warnings)
	}
}

func TestVoiceKiro_RemovesLiteralParentWhileLanguageStays(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".kiro", "settings", "cli.json")
	settings := filepath.Join(source, "settings", "voice.yaml")
	mustWriteGlobalTest(t, settings, "voice:\n  language: es\nx-kiro:\n  voice: {}\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, settings, "voice:\n  language: es\n")
	if _, warnings, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("drop literal voice object: %v\n%s", err, warnings)
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &values); err != nil {
		t.Fatal(err)
	}
	if _, present := values["voice"]; present {
		t.Error("unwanted literal voice object remains")
	}
	if values["voice.language"] != "es" {
		t.Errorf("language = %v", values["voice.language"])
	}
}
