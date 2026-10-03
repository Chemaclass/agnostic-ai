package adapters

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestNeutralHookScripts_EveryHookTargetCopiesAndReferencesSharedScript(t *testing.T) {
	cases := []struct{ target, event, output, scripts string }{
		{"claude", "PreToolUse", ".claude/settings.json", ".claude/hooks"},
		{"codex", "PreToolUse", ".codex/hooks.json", ".codex/hooks"},
		{"cursor", "beforeShellExecution", ".cursor/hooks.json", ".cursor/hooks"},
		{"gemini", "BeforeTool", ".gemini/settings.json", ".gemini/hooks"},
		{"qoder", "PreToolUse", ".qoder/settings.json", ".qoder/hooks"},
		{"factory", "PreToolUse", ".factory/hooks.json", ".factory/hooks"},
		{"trae", "PreToolUse", ".trae/hooks.json", ".trae/hooks"},
		{"openhands", "PreToolUse", ".openhands/hooks.json", ".openhands/hooks"},
		{"windsurf", "PreToolUse", ".devin/hooks.v1.json", ".devin/hooks"},
		{"antigravity", "PreInvocation", ".agents/hooks.json", ".agents/hooks"},
		{"copilot", "PreToolUse", ".github/hooks/agnostic-ai.json", ".github/hooks/scripts"},
		{"augment", "PreToolUse", ".augment/settings.json", ".augment/hooks"},
		{"crush", "PreToolUse", "crush.json", ".crush/hooks"},
		{"cline", "PreToolUse", ".clinerules/hooks/PreToolUse", ".cline/hooks/scripts"},
		{"kiro", "fileEdited", ".kiro/hooks/guard.json", ".kiro/scripts"},
		{"opencode", "PreToolUse", ".opencode/plugins/guard.ts", ".opencode/hooks"},
		{"kilo", "PreToolUse", ".kilo/plugin/guard.ts", ".kilo/hooks"},
		{"goose", "PreToolUse", ".agents/plugins/agnostic-ai/hooks/hooks.json", ".agents/plugins/agnostic-ai/hooks"},
		{"zed", "OnDemand", ".zed/tasks.json", ".zed/hooks"},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
				t.Fatal(err)
			}
			const body = "#!/bin/sh\nprintf '%s\\n' shared-hook\n"
			if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Targets: []string{c.target}, Outputs: map[string]config.Output{"zed": {TasksFile: ".zed/tasks.json"}}}
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": c.event, "command": ".agnostic-ai/scripts/guard.sh"}}
			adapter, ok := Get(c.target)
			if !ok {
				t.Fatalf("missing %s adapter", c.target)
			}
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), cfg, false); err != nil {
				t.Fatal(err)
			}
			script := filepath.ToSlash(filepath.Join(c.scripts, "guard.sh"))
			got, err := os.ReadFile(script)
			if err != nil {
				t.Errorf("shared script missing at %s: %v", script, err)
			} else if string(got) != body {
				t.Errorf("script body changed: %q", got)
			}
			native, err := os.ReadFile(c.output)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(native), script) || strings.Contains(string(native), ".agnostic-ai/scripts/") {
				t.Errorf("native hook has no working target script path %s:\n%s", script, native)
			}
		})
	}
}

func TestNeutralHookScripts_IgnoresCommandsOnNativeNonCommandHandlers(t *testing.T) {
	cases := []struct{ target, event, handler string }{
		{"claude", "PreToolUse", "http"},
		{"claude", "PreToolUse", "prompt"},
		{"qoder", "PreToolUse", "http"},
		{"qoder", "PreToolUse", "prompt"},
		{"copilot", "PreToolUse", "http"},
		{"copilot", "PreToolUse", "prompt"},
		{"cursor", "beforeShellExecution", "prompt"},
		{"windsurf", "PreToolUse", "prompt"},
	}
	for _, c := range cases {
		t.Run(c.target+"/"+c.handler, func(t *testing.T) {
			t.Chdir(t.TempDir())
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
				"event": c.event, "type": c.handler, "url": "https://example.test/hook", "prompt": "Check this edit",
				"command": ".agnostic-ai/scripts/missing.sh",
			}}
			adapter, _ := Get(c.target)
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{Targets: []string{c.target}}, false); err != nil {
				t.Errorf("unused command failed: %v", err)
			}
		})
	}
}

func TestNeutralHookScripts_GooseFollowsConfiguredPluginDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "#!/bin/sh\necho shared-hook\n"
	if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	const dir = "plugins/review plugin/hooks"
	cfg := &config.Config{Outputs: map[string]config.Output{"goose": {HooksFile: dir + "/hooks.json"}}}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard.sh"}}
	adapter, _ := Get("goose")
	if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), cfg, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dir + "/guard.sh"); err != nil || string(got) != body {
		t.Errorf("configured plugin script = %q, %v", got, err)
	}
	raw, err := os.ReadFile(dir + "/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Hooks["PreToolUse"][0].Hooks[0].Command
	if got != "export AGNOSTIC_AI_TARGET=goose; 'plugins/review plugin/hooks'/guard.sh" {
		t.Errorf("configured plugin command = %q", got)
	}
}

func TestNeutralHookScripts_GooseExecutesQuotedSourceAndDestinationPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	cases := []struct{ name, command, dir string }{
		{"guard.sh", ".agnostic-ai/scripts/guard.sh", "plugins/review;plugin/hooks"},
		{"guard.sh", ".agnostic-ai/scripts/guard.sh", "plugins/review[abc]*/hooks"},
		{"my guard.sh", "sh '.agnostic-ai/scripts/my guard.sh'", "plugins/review'&plugin/hooks"},
		{"my;guard.sh", "sh \".agnostic-ai/scripts/my;guard.sh\"", "plugins/review$`plugin/hooks"},
		{"my guard.sh", `sh .agnostic-ai/scripts/my\ guard.sh`, "plugins/review/hooks"},
	}
	for _, c := range cases {
		t.Run(c.command+"/"+c.dir, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
				t.Fatal(err)
			}
			const body = "#!/bin/sh\nprintf '%s\\n' shared-hook\n"
			if err := os.WriteFile(filepath.Join(".agnostic-ai/scripts", c.name), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": c.command}}
			cfg := &config.Config{Outputs: map[string]config.Output{"goose": {HooksFile: c.dir + "/hooks.json"}}}
			adapter, _ := Get("goose")
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), cfg, false); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(c.dir, c.name)); err != nil || string(got) != body {
				t.Errorf("materialized script = %q, %v", got, err)
			}
			raw, err := os.ReadFile(c.dir + "/hooks.json")
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Hooks map[string][]struct{ Hooks []struct{ Command string } }
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			command := doc.Hooks["PreToolUse"][0].Hooks[0].Command
			if got, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil || string(got) != "shared-hook\n" {
				t.Errorf("execute %q = %q, %v", command, got, err)
			}
		})
	}
}

func TestNeutralHookScripts_ClaudeExecFormKeepsLiteralFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable POSIX scripts")
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	const name = "my guard;script.sh"
	const body = "#!/bin/sh\nprintf '%s\\n' \"$1\"\n"
	if err := os.WriteFile(filepath.Join(".agnostic-ai/scripts", name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/" + name, "args": []any{"argument with ; punctuation"}}}
	adapter, _ := Get("claude")
	if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string
				Args    []string
			}
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	native := doc.Hooks["PreToolUse"][0].Hooks[0]
	if got, err := exec.Command(native.Command, native.Args...).CombinedOutput(); err != nil || string(got) != "argument with ; punctuation\n" {
		t.Errorf("execute %q %q = %q, %v", native.Command, native.Args, got, err)
	}
}

func TestNeutralHookScripts_GeminiNativeCommandOverrideCopiesItsScript(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte("echo native\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "BeforeTool", "command": "echo fallback", "x-gemini": map[string]any{"command": ".agnostic-ai/scripts/guard.sh"}}}
	adapter, _ := Get("gemini")
	if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(".gemini/hooks/guard.sh"); err != nil || string(got) != "echo native\n" {
		t.Errorf("native override script = %q, %v", got, err)
	}
}

func TestNeutralHookScripts_NativeOverridesIgnoreParentArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, target := range []string{"gemini", "kiro"} {
		t.Run(target, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
				t.Fatal(err)
			}
			const body = "#!/bin/sh\nprintf '%s\\n' \"$1\"\n"
			if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			const command = "sh .agnostic-ai/scripts/guard.sh --strict"
			meta := map[string]any{"event": "BeforeTool", "command": ".agnostic-ai/scripts/missing.sh", "args": []any{"ignored-parent-argument"}}
			output := ".gemini/settings.json"
			if target == "gemini" {
				meta["x-gemini"] = map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
			} else {
				meta["event"] = "fileEdited"
				meta["x-kiro"] = map[string]any{"action": map[string]any{"type": "command", "command": command}}
				output = ".kiro/hooks/guard.json"
			}
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: meta}
			adapter, _ := Get(target)
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Hooks json.RawMessage
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			var nativeCommand string
			if target == "gemini" {
				var events map[string][]struct{ Hooks []struct{ Command string } }
				if err := json.Unmarshal(doc.Hooks, &events); err != nil {
					t.Fatal(err)
				}
				nativeCommand = events["BeforeTool"][0].Hooks[0].Command
			} else {
				var hooks []struct{ Action struct{ Command string } }
				if err := json.Unmarshal(doc.Hooks, &hooks); err != nil {
					t.Fatal(err)
				}
				nativeCommand = hooks[0].Action.Command
			}
			if got, err := exec.Command("sh", "-c", nativeCommand).CombinedOutput(); err != nil || string(got) != "--strict\n" {
				t.Errorf("execute native override %q = %q, %v", nativeCommand, got, err)
			}
		})
	}
}

func TestNeutralHookScripts_MixedVendorReferenceStillCopiesItsScript(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts/claude", 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{".agnostic-ai/scripts/pre.sh": "echo shared\n", ".agnostic-ai/scripts/claude/post.sh": "echo vendor\n"} {
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "sh .agnostic-ai/scripts/pre.sh && sh .claude/hooks/post.sh"}}
	adapter, _ := Get("codex")
	if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{".codex/hooks/pre.sh": "echo shared\n", ".codex/hooks/post.sh": "echo vendor\n"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != body {
			t.Errorf("mixed command script %s = %q, %v", path, got, err)
		}
	}
}

func TestNeutralHookScripts_KiroNativeDisabledActionKeepsWorkingScriptPath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte("echo guard\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "fileEdited", "disabled": true, "x-kiro": map[string]any{"action": map[string]any{"type": "command", "command": ".agnostic-ai/scripts/guard.sh"}}}}
	adapter, _ := Get("kiro")
	if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(".kiro/scripts/guard.sh"); err != nil || string(got) != "echo guard\n" {
		t.Errorf("Kiro script = %q, %v", got, err)
	}
	if got, err := os.ReadFile(".kiro/hooks/guard.json"); err != nil || !strings.Contains(string(got), `"command": ".kiro/scripts/guard.sh"`) || !strings.Contains(string(got), `"enabled": false`) {
		t.Errorf("Kiro action = %s, %v", got, err)
	}
}
