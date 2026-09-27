package claude

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type claudeTargetDoc struct {
	Env   map[string]string `json:"env"`
	Hooks map[string][]struct {
		Hooks []map[string]any `json:"hooks"`
	} `json:"hooks"`
}

func readClaudeTargetDoc(t *testing.T) claudeTargetDoc {
	t.Helper()
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc claudeTargetDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// Claude Code has no per-hook env, and a command prefix would miss exec
// form and PowerShell. The settings `env` reaches every hook it starts.
func TestEmit_CommandHooksSetTheTargetInSettingsEnv(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "shell", Meta: map[string]any{"event": "PreToolUse", "command": "guard.sh"}},
		{Kind: spec.KindHook, Name: "exec", Meta: map[string]any{"event": "PreToolUse", "command": "node", "args": []any{"guard.js"}}},
	}
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Settings: &config.ClaudeSettings{Env: map[string]string{"FOO": "bar"}}}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	doc := readClaudeTargetDoc(t)
	if doc.Env[emit.HookTargetEnv] != "claude" || doc.Env["FOO"] != "bar" {
		t.Errorf("env = %v", doc.Env)
	}
	handlers := doc.Hooks["PreToolUse"][0].Hooks
	if handlers[0]["command"] != "guard.sh" || handlers[1]["command"] != "node" {
		t.Errorf("commands changed: %v", handlers)
	}
}

func TestEmit_ConfigEnvCanPinTheTargetVariable(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindHook, Name: "shell", Meta: map[string]any{"event": "PreToolUse", "command": "guard.sh"}}}
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Settings: &config.ClaudeSettings{Env: map[string]string{emit.HookTargetEnv: "mine"}}}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeTargetDoc(t).Env[emit.HookTargetEnv]; got != "mine" {
		t.Errorf("%s = %q, want the config value", emit.HookTargetEnv, got)
	}
}

func TestEmit_DropsTheTargetVariableWithTheLastCommandHook(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"env": {"AGNOSTIC_AI_TARGET": "claude"}, "hooks": {"Stop": []}}` + "\n"
	if err := os.WriteFile(".claude/settings.json", []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindHook, Name: "http", Meta: map[string]any{"event": "Stop", "type": "http", "url": "https://example.test/stop"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if env := readClaudeTargetDoc(t).Env; env != nil {
		t.Errorf("env = %v, want it gone with no command hook left", env)
	}
}

func TestEmit_ConfigPinnedTargetSurvivesWithoutCommandHooks(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"claude": {Settings: &config.ClaudeSettings{Env: map[string]string{emit.HookTargetEnv: "claude", "OTHER": "1"}}}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if env := readClaudeTargetDoc(t).Env; env[emit.HookTargetEnv] != "claude" || env["OTHER"] != "1" {
		t.Errorf("env = %v, want the pinned value kept", env)
	}
}

func TestEmit_TargetEnvKeepsTheUsersKeyOrder(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".claude/settings.json", []byte(`{"env": {"ZED": "1", "ALPHA": "2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindHook, Name: "stop", Meta: map[string]any{"event": "Stop", "command": "done.sh"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	zed, alpha, target := strings.Index(string(data), `"ZED"`), strings.Index(string(data), `"ALPHA"`), strings.Index(string(data), `"AGNOSTIC_AI_TARGET"`)
	if zed < 0 || zed > alpha || alpha > target {
		t.Errorf("env keys reordered:\n%s", data)
	}
}
