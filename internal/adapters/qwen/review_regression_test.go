package qwen

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func qwenSettings(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEmit_SettingsNativeGuardsSurviveGeneratedHooks(t *testing.T) {
	testutil.TempCwd(t)
	generated := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo generated", "env": map[string]any{"AGNOSTIC_AI_TARGET": "qwen"}}}}
	guard := map[string]any{"matcher": "run_shell_command", "hooks": []any{map[string]any{"type": "command", "command": "native-guard"}}}
	bundle := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "generated", Meta: map[string]any{"event": "PreToolUse", "command": "echo generated"}},
		{Kind: spec.KindSettings, Name: "native", Meta: map[string]any{"x-qwen": map[string]any{"disableAllHooks": true, "hooks": map[string]any{"enabled": false, "PreToolUse": []any{guard, generated}}}}},
	})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	doc := qwenSettings(t)
	hooks := doc["hooks"].(map[string]any)
	groups := hooks["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("native and generated guards must each appear once, got %#v", groups)
	}
	if doc["disableAllHooks"] != true || hooks["enabled"] != false {
		t.Errorf("native settings lost: %#v", doc)
	}
	if groups[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] != "native-guard" {
		t.Errorf("native guard lost or reordered: %#v", groups)
	}
	first, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(defaultSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("repeated emit changed hooks:\n%s\n%s", first, second)
	}
}

func TestEmit_NativeMCPRecordsSurviveAndOverridePortableRecords(t *testing.T) {
	testutil.TempCwd(t)
	bundle := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "portable", Meta: map[string]any{"command": "portable-server"}},
		{Kind: spec.KindMCP, Name: "clash", Meta: map[string]any{"type": "http", "url": "https://example.test/portable", "headers": map[string]any{"X-Portable": "yes"}}},
		{Kind: spec.KindSettings, Name: "native", Meta: map[string]any{"x-qwen": map[string]any{"mcpServers": map[string]any{"native": map[string]any{"command": "native-server"}, "clash": map[string]any{"command": "native-override"}}}}},
	})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	servers := qwenSettings(t)["mcpServers"].(map[string]any)
	if len(servers) != 3 {
		t.Errorf("distinct MCP records lost: %#v", servers)
	}
	clash := servers["clash"].(map[string]any)
	if len(clash) != 1 || clash["command"] != "native-override" {
		t.Errorf("native override did not replace the whole portable record: %#v", clash)
	}
}

func TestEmit_NativeSkillModelSurvivesAndPortableModelStaysUnsupported(t *testing.T) {
	testutil.TempCwd(t)
	bundle := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSkill, Name: "native", Body: "Check", Meta: map[string]any{"model": "portable-model", "x-qwen": map[string]any{"model": "custom-model"}}},
		{Kind: spec.KindSkill, Name: "portable", Body: "Check", Meta: map[string]any{"model": "portable-model"}},
	})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	native, err := os.ReadFile(".qwen/skills/native/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(native), "model: custom-model") {
		t.Errorf("native model lost: %s", native)
	}
	portable, err := os.ReadFile(".qwen/skills/portable/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(portable), "model:") {
		t.Errorf("portable model was promoted: %s", portable)
	}
}

func TestEmit_OrdinaryPortableHooksProduceNativeBlockingReplies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes POSIX hook commands")
	}
	for _, on := range []string{"before-tool", "prompt-submit", "after-tool", "after-edit", "stop"} {
		for _, command := range []string{`printf '{}' >&2; exit 2`, `exit 2`} {
			t.Run(on+"/"+command, func(t *testing.T) {
				testutil.TempCwd(t)
				hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"on": on, "command": command}}
				native, reason := hook.NativeHook(target)
				if reason != "" {
					t.Fatal(reason)
				}
				if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{native}), &config.Config{}, false); err != nil {
					t.Fatal(err)
				}
				event := native.Meta["event"].(string)
				handler := qwenSettings(t)["hooks"].(map[string]any)[event].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
				cmd := exec.Command("bash", "-c", handler["command"].(string))
				var stderr strings.Builder
				cmd.Stderr = &stderr
				stdout, err := cmd.Output()
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 2 || len(stdout) != 0 {
					t.Fatalf("exit=%v stdout=%q", err, stdout)
				}
				var reply map[string]any
				if err := json.Unmarshal([]byte(stderr.String()), &reply); err != nil {
					t.Fatalf("no native blocking reply: %v, stderr=%q", err, stderr.String())
				}
				if on == "after-tool" || on == "after-edit" {
					if reply["continue"] != false {
						t.Errorf("PostToolUse only consumes continue:false: %#v", reply)
					}
				} else if reply["decision"] != "deny" {
					t.Errorf("native event did not receive a denial: %#v", reply)
				}
			})
		}
	}
	for _, on := range []string{"session-start", "session-end"} {
		native, reason := (spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"on": on, "command": "exit 2"}}).NativeHook(target)
		if reason != "" {
			t.Fatal(reason)
		}
		if native.WrapsCommand(target) {
			t.Errorf("%s cannot block and must retain its native lifecycle behavior", on)
		}
	}
}
