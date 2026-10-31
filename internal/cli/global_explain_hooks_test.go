package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainGlobal_DirectoryHookFilesMatchSync(t *testing.T) {
	for _, target := range []string{"kiro", "grok", "cline"} {
		for _, custom := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/custom=%t", target, custom), func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+target+"]\n")
				root := filepath.Join(home, "."+target)
				if custom {
					root = filepath.Join(home, "custom "+target)
					t.Setenv(globalTargets[target].rootEnv, root)
				}
				name := "guard.json"
				event := "PreToolUse"
				if target == "cline" {
					name, event = "PreToolUse", "pretooluse"
				}
				path := filepath.Join(root, "hooks", name)
				mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: "+event+"\ncommand: echo guard\n")
				out, err := runRoot(t, "explain", "--global", "hooks/guard.yaml", "--json")
				if err != nil {
					t.Fatal(err)
				}
				var got explainOutput
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("parse explain: %v\n%s", err, out)
				}
				if len(got.Contributions) != 1 || got.Contributions[0] != (contribution{Target: target, Path: path, Mode: "full"}) {
					t.Errorf("explain contributions = %+v, want %s", got.Contributions, path)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("explain wrote native hook: %v", err)
				}
				text, err := runRoot(t, "explain", "--global", "hooks/guard.yaml")
				if err != nil || !strings.Contains(text, filepath.ToSlash(path)+" (full file)") {
					t.Errorf("explain text: %v\n%s", err, text)
				}
				preview, _, err := runGlobalAgentTest("--only", target, "--dry-run")
				if err != nil || !strings.Contains(preview, path) {
					t.Fatalf("sync preview: %v\n%s", err, preview)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("dry-run wrote native hook: %v", err)
				}
				if out, _, err := runGlobalAgentTest("--only", target); err != nil {
					t.Fatalf("sync: %v\n%s", err, out)
				}
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), "guard") {
					t.Errorf("explain output not written by sync: %v\n%s", err, data)
				}
			})
		}
	}
}

func TestExplainGlobal_DirectoryHooksWithNoNativeOutput(t *testing.T) {
	for _, tc := range []struct {
		name, target, hook string
	}{
		{"kiro unsupported portable event", "kiro", "on: after-tool\ncommand: echo guard\n"},
		{"cline unsupported event", "cline", "event: UnknownEvent\ncommand: echo guard\n"},
		{"cline project-only runtime", "cline", "event: PreToolUse\ncommand: echo guard\nx-cline:\n  runtime: vscode\n"},
		{"kiro without command", "kiro", "event: PreToolUse\n"},
		{"grok without command", "grok", "event: PreToolUse\n"},
		{"cline without command", "cline", "event: PreToolUse\n"},
		{"kiro target excluded", "kiro", "event: PreToolUse\ncommand: echo guard\ntargets: [claude]\n"},
		{"grok target excluded", "grok", "event: PreToolUse\ncommand: echo guard\ntargets: [claude]\n"},
		{"cline target excluded", "cline", "event: PreToolUse\ncommand: echo guard\ntargets: [claude]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+tc.target+"]\n")
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\n"+tc.hook)
			out, err := runRoot(t, "explain", "--global", "hooks/guard.yaml", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var got explainOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Contributions) != 0 {
				t.Errorf("explain claims outputs sync omits: %+v", got.Contributions)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			dir := globalTargets[tc.target].path(home, globalTargets[tc.target].hooksDir)
			if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
				t.Errorf("sync unexpectedly wrote native hooks: %v", entries)
			}
		})
	}
}

func TestExplainGlobal_DirectoryHookNativeActions(t *testing.T) {
	for _, tc := range []struct {
		name, target, hook string
	}{
		{"Kiro agent action", "kiro", "event: Stop\nx-kiro:\n  action:\n    type: agent\n    prompt: Review the work.\n"},
		{"Grok native handlers", "grok", "x-grok:\n  hooks:\n    Stop:\n      - hooks:\n          - type: command\n            command: echo guard\n"},
		{"Kiro disabled hook", "kiro", "event: PreToolUse\ncommand: echo guard\ndisabled: true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+tc.target+"]\n")
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\n"+tc.hook)
			path := filepath.Join(home, "."+tc.target, "hooks", "guard.json")
			out, err := runRoot(t, "explain", "--global", "hooks/guard.yaml")
			if err != nil || !strings.Contains(out, filepath.ToSlash(path)) {
				t.Errorf("explain omits native action: %v\n%s", err, out)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("sync omitted native hook: %v", err)
			}
		})
	}
}

func TestExplainGlobal_DirectoryHookScriptsMatchSync(t *testing.T) {
	for _, target := range []string{"kiro", "grok", "cline"} {
		t.Run(target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+target+"]\n")
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: [.agnostic-ai/scripts/guard.ps1, .agnostic-ai/scripts/guard.ps1]\n")
			body := "exit 97\n"
			mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.ps1"), body)
			out, err := runRoot(t, "explain", "--global", "hooks/guard.yaml", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var got explainOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Contributions) != 2 {
				t.Errorf("explain contributions = %+v, want one native hook and one copied script", got.Contributions)
			}
			script := filepath.Join(globalTargets[target].path(home, globalTargets[target].hookScriptsDir), "guard.ps1")
			found := false
			for _, c := range got.Contributions {
				if c.Path == script {
					found = true
				}
				if _, err := os.Stat(c.Path); !os.IsNotExist(err) {
					t.Errorf("explain wrote %s: %v", c.Path, err)
				}
			}
			if !found {
				t.Errorf("explain omitted copied script: %+v", got.Contributions)
			}
			if _, _, err := runGlobalAgentTest("--only", target); err != nil {
				t.Fatal(err)
			}
			for _, c := range got.Contributions {
				if _, err := os.Stat(c.Path); err != nil {
					t.Errorf("sync did not write explained file %s: %v", c.Path, err)
				}
			}
			if data, err := os.ReadFile(script); err != nil || string(data) != body {
				t.Errorf("copied script = %q, %v", data, err)
			}
		})
	}
}

func TestExplainGlobal_DirectoryHookValidationMatchesSync(t *testing.T) {
	for _, tc := range []struct {
		target, hook, want string
	}{
		{"grok", "event: UnknownEvent\ncommand: echo guard\n", "unsupported event"},
		{"kiro", "event: PreToolUse\nx-kiro:\n  action:\n    type: unknown\n", "action.type"},
		{"kiro", "event: PreToolUse\ncommand: .agnostic-ai/scripts/missing.sh\n", "shared hook script not found"},
	} {
		t.Run(tc.target+tc.want, func(t *testing.T) {
			_, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+tc.target+"]\n")
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\n"+tc.hook)
			if out, err := runRoot(t, "explain", "--global", "hooks/guard.yaml"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("explain error = %v, want %q\n%s", err, tc.want, out)
			}
			if out, _, err := runGlobalAgentTest("--only", tc.target); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("sync error = %v, want %q\n%s", err, tc.want, out)
			}
		})
	}
}
