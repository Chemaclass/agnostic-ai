package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncGlobal_NeutralHookExecutesWithShellPunctuationInHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	home, source := globalAgentTestHome(t)
	home = filepath.Join(home, "user;home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), "#!/bin/sh\nprintf '%s\\n' shared-hook\n")
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: SessionStart\ncommand: .agnostic-ai/scripts/guard.sh\n")
	if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	doc := readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json"))
	command := firstGlobalHandler(t, doc, "SessionStart")["command"].(string)
	if got, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil || string(got) != "shared-hook\n" {
		t.Errorf("execute %q = %q, %v", command, got, err)
	}
}

func TestSyncGlobal_NeutralHookExecFormKeepsLiteralFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable POSIX scripts")
	}
	home, source := globalAgentTestHome(t)
	home = filepath.Join(home, "user;home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	const name = "my guard;script.sh"
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", name), "#!/bin/sh\nprintf '%s\\n' \"$1\"\n")
	if err := os.Chmod(filepath.Join(source, "scripts", name), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: SessionStart\ncommand: .agnostic-ai/scripts/"+name+"\nargs: ['argument with ; punctuation']\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
		t.Fatal(err)
	}
	claude := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".claude", "settings.json")), "SessionStart")
	args := stringSliceFromAny(claude["args"])
	if got, err := exec.Command(claude["command"].(string), args...).CombinedOutput(); err != nil || string(got) != "argument with ; punctuation\n" {
		t.Errorf("execute Claude exec form = %q, %v", got, err)
	}
	codex := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if got, err := exec.Command("sh", "-c", codex["command"].(string)).CombinedOutput(); err != nil || string(got) != "argument with ; punctuation\n" {
		t.Errorf("execute Codex folded exec form = %q, %v", got, err)
	}
}

func TestSyncGlobal_NeutralHookScriptsCopyToUserTargets(t *testing.T) {
	home, source := globalAgentTestHome(t)
	const body = "#!/bin/sh\necho shared-hook\n"
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), body)
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		event := "SessionStart"
		if target == "cursor" {
			event = "sessionStart"
		}
		mustWriteGlobalTest(t, filepath.Join(source, "hooks", target+".yaml"), "name: guard-"+target+"\nevent: "+event+"\ntarget: "+target+"\ncommand: .agnostic-ai/scripts/guard.sh\n")
	}
	only := []string{"--only", "claude,codex,cursor,gemini,qoder,augment"}
	if _, _, err := runGlobalAgentTest(only...); err != nil {
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
		path := globalTargets[target].path(home, globalTargets[target].hooks)
		native, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(native), filepath.ToSlash(script)) || strings.Contains(string(native), ".agnostic-ai/scripts/") {
			t.Errorf("%s native hook has no user script path:\n%s", target, native)
		}
	}
	if _, _, err := runGlobalAgentTest(append(only, "--check")...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		if err := os.Remove(filepath.Join(source, "hooks", target+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runGlobalAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "qoder", "augment"} {
		if _, err := os.Stat(filepath.Join(home, "."+target, "hooks", "guard.sh")); !os.IsNotExist(err) {
			t.Errorf("%s deleted hook left its script behind: %v", target, err)
		}
	}
}

func TestSyncGlobal_WindowsHookUsesCopiedTargetScript(t *testing.T) {
	for _, tc := range []struct {
		name, command, prefix, suffix string
		args                          string
	}{
		{name: "script path", command: ".agnostic-ai/scripts/guard.ps1"},
		{name: "PowerShell command", command: `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, prefix: `pwsh -File "`, suffix: `" --strict`},
		{name: "separate POSIX arguments", command: `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, prefix: `pwsh -File "`, suffix: `" --strict`, args: "args: ['--posix-only']\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.ps1"), "Write-Output 'shared'\n")
			const variant = "Write-Output 'codex'\n"
			mustWriteGlobalTest(t, filepath.Join(source, "scripts", "codex", "guard.ps1"), variant)
			hook := filepath.Join(source, "hooks", "guard.yaml")
			mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: 'true'\ncommandWindows: '"+tc.command+"'\n"+tc.args)
			if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
				t.Fatal(err)
			}
			handler := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
			script := filepath.Join(home, ".codex", "hooks", "guard.ps1")
			if got, want := handler["commandWindows"], tc.prefix+filepath.ToSlash(script)+tc.suffix; got != want {
				t.Errorf("commandWindows = %q, want %q", got, want)
			}
			copied, err := os.ReadFile(script)
			if err != nil {
				t.Errorf("Windows script was not copied: %v", err)
			} else if string(copied) != variant {
				t.Errorf("copied Windows script = %q, want target variant", copied)
			}
			if _, _, err := runGlobalAgentTest("--only", "codex", "--check"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(script); !os.IsNotExist(err) {
				t.Errorf("deleted hook left Windows script behind: %v", err)
			}
		})
	}
}
