package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKiroHome_BuiltBinaryUsesFallbackAndKeepsUserFiles(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, profile := range []string{"relative", "home", "ancestor", "valid"} {
		t.Run(profile, func(t *testing.T) {
			base := t.TempDir()
			home, source := filepath.Join(base, "home"), filepath.Join(base, "source")
			root := filepath.Join(home, ".kiro")
			override := "profiles/work"
			switch profile {
			case "home":
				override = home
			case "ancestor":
				override = base
			case "valid":
				override = filepath.Join(base, "profile")
				root = override
			}
			for key, value := range map[string]string{"HOME": home, "USERPROFILE": home, "AGNOSTIC_AI_HOME": source, "KIRO_HOME": override, "XDG_CONFIG_HOME": filepath.Join(home, ".config"), "AGNOSTIC_AI_TARGET": "", "AGNOSTIC_AI_NO_UPDATE_CHECK": "1"} {
				t.Setenv(key, value)
			}
			write := func(path, body string) {
				t.Helper()
				must(t, os.MkdirAll(filepath.Dir(path), 0o755))
				must(t, os.WriteFile(path, []byte(body), 0o600))
			}
			run := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = base
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			write(filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro]\n")
			native := `{"version":"v1","hooks":[{"name":"guard","trigger":"PreToolUse","action":{"type":"command","command":"guard.sh"}}]}`
			hook := filepath.Join(root, "hooks", "guard.json")
			write(hook, native)
			settingsPath := filepath.Join(root, "settings", "cli.json")
			originalSettings := `{ "voice.language" : "en", "voice.autoSubmit": false }`
			write(settingsPath, originalSettings)
			run("import", "--global", "kiro")
			importedSettings := filepath.Join(source, "settings", "imported.yaml")
			if data, err := os.ReadFile(importedSettings); err != nil || !strings.Contains(string(data), "language: en") {
				t.Fatalf("voice import did not use selected root: %s: %v", data, err)
			}
			if data, err := os.ReadFile(settingsPath); err != nil || string(data) != originalSettings {
				t.Fatalf("import changed settings: %q: %v", data, err)
			}
			write(filepath.Join(source, "settings", "voice.yaml"), "voice:\n  language: es\n")
			if data, err := os.ReadFile(filepath.Join(source, "hooks", "guard.yaml")); err != nil || !strings.Contains(string(data), "guard.sh") {
				t.Fatalf("import did not use selected root: %s: %v", data, err)
			}
			write(filepath.Join(source, "AGNOSTIC_AI.md"), "Use clear names.\n")
			manual := filepath.Join(root, "steering", "AGENTS.md")
			write(manual, "User instructions.\n")
			write(filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review changes\n---\nReview changes.\n")
			write(filepath.Join(source, "skills", "helper.md"), "---\nname: helper\ndescription: Help with changes\n---\nHelp with changes.\n")
			out := run("explain", "--global", "agents/reviewer.md")
			if !strings.Contains(out, filepath.ToSlash(filepath.Join(root, "agents", "reviewer.md"))) {
				t.Errorf("explain used another root: %s", out)
			}
			voiceExplain := run("explain", "--global", "settings/voice.yaml")
			if !strings.Contains(voiceExplain, filepath.ToSlash(settingsPath)) || !strings.Contains(voiceExplain, `key "voice.language"`) {
				t.Errorf("voice explain used another root: %s", voiceExplain)
			}
			run("sync", "--global", "--only", "kiro", "--backup")
			if data, err := os.ReadFile(settingsPath); err != nil || string(data) != strings.Replace(originalSettings, `"en"`, `"es"`, 1) {
				t.Errorf("voice sync changed unrelated bytes: %q: %v", data, err)
			}
			for _, path := range []string{manual, hook, filepath.Join(root, "agents", "reviewer.md"), filepath.Join(root, "skills", "helper", "SKILL.md")} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("expected output %s: %v", path, err)
				}
			}
			run("sync", "--global", "--only", "kiro", "--check")
			for _, path := range []string{"AGNOSTIC_AI.md", "hooks/guard.yaml", "agents/reviewer.md", "skills/helper.md", "settings/voice.yaml", "settings/imported.yaml"} {
				must(t, os.Remove(filepath.Join(source, filepath.FromSlash(path))))
			}
			run("sync", "--global", "--only", "kiro")
			if data, err := os.ReadFile(settingsPath); err != nil || string(data) != originalSettings {
				t.Errorf("voice removal failed to restore bytes: %q: %v", data, err)
			}
			if data, err := os.ReadFile(manual); err != nil || string(data) != "User instructions.\n" {
				t.Errorf("manual instructions changed: %q: %v", data, err)
			}
			for _, path := range []string{hook, filepath.Join(root, "agents", "reviewer.md"), filepath.Join(root, "skills", "helper", "SKILL.md")} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("removed source left %s: %v", path, err)
				}
			}
		})
	}
}
