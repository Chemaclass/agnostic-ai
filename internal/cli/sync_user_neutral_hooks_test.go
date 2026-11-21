package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncUser_AugmentMemoryIncludesItsExecutableBuiltinScript(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [augment]\nbuiltins: [memory]\n")
	if _, _, err := runUserAgentTest("--only", "augment"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".augment", "hooks", "memory-augment-session-start.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "#!/bin/sh\n") || !strings.Contains(string(data), "hook memory --target augment") {
		t.Errorf("global Augment script does not load memory: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("global Augment script is not executable: %o", info.Mode().Perm())
	}
	command := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".augment", "settings.json")), "SessionStart")["command"].(string)
	if command != filepath.ToSlash(path) {
		t.Errorf("global Augment script command = %q, want %q", command, filepath.ToSlash(path))
	}
	if _, _, err := runUserAgentTest("--only", "augment", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncUser_NeutralHookExecutesWithShellPunctuationInHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	home, source := userAgentTestHome(t)
	home = filepath.Join(home, "user;home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeFile(t, filepath.Join(source, "scripts", "guard.sh"), "#!/bin/sh\nprintf '%s\\n' shared-hook\n")
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "event: SessionStart\ncommand: .agnostic-ai/scripts/guard.sh\n")
	if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	doc := readUserJSON(t, filepath.Join(home, ".codex", "hooks.json"))
	command := firstUserHandler(t, doc, "SessionStart")["command"].(string)
	if got, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil || string(got) != "shared-hook\n" {
		t.Errorf("execute %q = %q, %v", command, got, err)
	}
}

func TestSyncUser_NeutralHookExecFormKeepsLiteralFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable POSIX scripts")
	}
	home, source := userAgentTestHome(t)
	home = filepath.Join(home, "user;home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	const name = "my guard;script.sh"
	writeFile(t, filepath.Join(source, "scripts", name), "#!/bin/sh\nprintf '%s\\n' \"$1\"\n")
	if err := os.Chmod(filepath.Join(source, "scripts", name), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "event: SessionStart\ncommand: .agnostic-ai/scripts/"+name+"\nargs: ['argument with ; punctuation']\n")
	if _, _, err := runUserAgentTest("--only", "claude,codex"); err != nil {
		t.Fatal(err)
	}
	claude := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".claude", "settings.json")), "SessionStart")
	args := stringSliceFromAny(claude["args"])
	if got, err := exec.Command(claude["command"].(string), args...).CombinedOutput(); err != nil || string(got) != "argument with ; punctuation\n" {
		t.Errorf("execute Claude exec form = %q, %v", got, err)
	}
	codex := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if got, err := exec.Command("sh", "-c", codex["command"].(string)).CombinedOutput(); err != nil || string(got) != "argument with ; punctuation\n" {
		t.Errorf("execute Codex folded exec form = %q, %v", got, err)
	}
}

func TestSyncUser_NeutralHookScriptsCopyToUserTargets(t *testing.T) {
	home, source := userAgentTestHome(t)
	const body = "#!/bin/sh\necho shared-hook\n"
	writeFile(t, filepath.Join(source, "scripts", "guard.sh"), body)
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		event := "SessionStart"
		if target == "cursor" {
			event = "sessionStart"
		}
		writeFile(t, filepath.Join(source, "hooks", target+".yaml"), "name: guard-"+target+"\nevent: "+event+"\ntarget: "+target+"\ncommand: .agnostic-ai/scripts/guard.sh\n")
	}
	only := []string{"--only", "claude,codex,cursor,gemini,qoder,augment"}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		script := filepath.Join(home, "."+target, "hooks", "guard.sh")
		got, err := os.ReadFile(script)
		if err != nil {
			t.Errorf("%s script missing: %v", target, err)
		} else if string(got) != body {
			t.Errorf("%s script body changed: %q", target, got)
		}
		path := userTargets[target].path(home, userTargets[target].hooks)
		native, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// A temp dir such as RUNNER~1 on Windows makes sync shell-quote the directory.
		unquoted := strings.ReplaceAll(string(native), "'", "")
		if !strings.Contains(unquoted, filepath.ToSlash(script)) || strings.Contains(unquoted, ".agnostic-ai/scripts/") {
			t.Errorf("%s native hook has no user script path:\n%s", target, native)
		}
	}
	if _, _, err := runUserAgentTest(append(only, "--check")...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		if err := os.Remove(filepath.Join(source, "hooks", target+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		if _, err := os.Stat(filepath.Join(home, "."+target, "hooks", "guard.sh")); !os.IsNotExist(err) {
			t.Errorf("%s deleted hook left its script behind: %v", target, err)
		}
	}
}

func TestSyncUser_WindowsHookUsesCopiedTargetScript(t *testing.T) {
	for _, tc := range []struct {
		name, command, prefix, suffix string
		args                          string
	}{
		{name: "script path", command: ".agnostic-ai/scripts/guard.ps1"},
		{name: "PowerShell command", command: `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, prefix: `pwsh -File "`, suffix: `" --strict`},
		{name: "project root prefix", command: `$s = "$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.ps1"; sh $s`, prefix: `$s = "`, suffix: `"; sh $s`},
		{name: "separate POSIX arguments", command: `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, prefix: `pwsh -File "`, suffix: `" --strict`, args: "args: ['--posix-only']\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			writeFile(t, filepath.Join(source, "scripts", "guard.ps1"), "Write-Output 'shared'\n")
			const variant = "Write-Output 'codex'\n"
			writeFile(t, filepath.Join(source, "scripts", "codex", "guard.ps1"), variant)
			hook := filepath.Join(source, "hooks", "guard.yaml")
			writeFile(t, hook, "event: SessionStart\ncommand: 'true'\ncommandWindows: '"+tc.command+"'\n"+tc.args)
			if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
				t.Fatal(err)
			}
			handler := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
			script := filepath.Join(home, ".codex", "hooks", "guard.ps1")
			if got, want := handler["commandWindows"], tc.prefix+script+tc.suffix; got != want {
				t.Errorf("commandWindows = %q, want %q", got, want)
			}
			copied, err := os.ReadFile(script)
			if err != nil {
				t.Errorf("Windows script was not copied: %v", err)
			} else if string(copied) != variant {
				t.Errorf("copied Windows script = %q, want target variant", copied)
			}
			if _, _, err := runUserAgentTest("--only", "codex", "--check"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(script); !os.IsNotExist(err) {
				t.Errorf("deleted hook left Windows script behind: %v", err)
			}
		})
	}
}

// Auggie hands a command without args to a shell, so a shared script
// path in a home with spaces reaches it quoted.
func TestSyncUser_AugmentNeutralHookExecutesFromHomeWithSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	home, source := userAgentTestHome(t)
	home = filepath.Join(home, "user home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeFile(t, filepath.Join(source, "scripts", "guard.sh"), "#!/bin/sh\nprintf '%s\\n' shared-hook\n")
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "event: SessionStart\ncommand: .agnostic-ai/scripts/guard.sh\n")
	if _, _, err := runUserAgentTest("--only", "augment"); err != nil {
		t.Fatal(err)
	}
	command := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".augment", "settings.json")), "SessionStart")["command"].(string)
	if got, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil || string(got) != "shared-hook\n" {
		t.Errorf("execute %q = %q, %v", command, got, err)
	}
}
