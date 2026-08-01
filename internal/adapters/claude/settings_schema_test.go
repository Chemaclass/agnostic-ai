package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func TestEmit_FirstClassSettings_AllFields(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {
				Settings: &config.ClaudeSettings{
					Model:               "claude-opus-4-7",
					OutputStyle:         "verbose",
					APIKeyHelper:        "./bin/keyhelper.sh",
					CleanupPeriodDays:   intPtr(30),
					IncludeCoAuthoredBy: boolPtr(false),
					EnabledPlugins:      map[string]bool{"plugin-a@marketplace": true, "plugin-b@marketplace": true},
					Env:                 map[string]string{"FOO": "bar", "BAZ": "qux"},
					StatusLine: &config.ClaudeStatusLine{
						Type:    "command",
						Command: "echo status",
						Padding: intPtr(2),
					},
					Permissions: &config.ClaudePermissions{
						Allow: []string{"Read(*)"},
						Deny:  []string{"Shell(rm *)"},
						Ask:   []string{"Edit(*)"},
					},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json from config alone: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if got := parsed["model"]; got != "claude-opus-4-7" {
		t.Errorf("model: got %v", got)
	}
	if got := parsed["outputStyle"]; got != "verbose" {
		t.Errorf("outputStyle: got %v", got)
	}
	if got := parsed["apiKeyHelper"]; got != "./bin/keyhelper.sh" {
		t.Errorf("apiKeyHelper: got %v", got)
	}
	if got := parsed["cleanupPeriodDays"]; got != float64(30) {
		t.Errorf("cleanupPeriodDays: got %v", got)
	}
	if got := parsed["includeCoAuthoredBy"]; got != false {
		t.Errorf("includeCoAuthoredBy: got %v", got)
	}
	plugins, ok := parsed["enabledPlugins"].(map[string]any)
	if !ok || len(plugins) != 2 || plugins["plugin-a@marketplace"] != true || plugins["plugin-b@marketplace"] != true {
		t.Errorf("enabledPlugins wrong: %v", parsed["enabledPlugins"])
	}
	env, ok := parsed["env"].(map[string]any)
	if !ok || env["FOO"] != "bar" {
		t.Errorf("env wrong: %v", parsed["env"])
	}
	sl, ok := parsed["statusLine"].(map[string]any)
	if !ok || sl["command"] != "echo status" || sl["padding"] != float64(2) {
		t.Errorf("statusLine wrong: %v", parsed["statusLine"])
	}
	perms, ok := parsed["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions missing: %v", parsed["permissions"])
	}
	for key, want := range map[string]string{"allow": "Read(*)", "deny": "Shell(rm *)", "ask": "Edit(*)"} {
		list, ok := perms[key].([]any)
		if !ok || len(list) != 1 || list[0] != want {
			t.Errorf("permissions.%s wrong: %v", key, perms[key])
		}
	}
}

func TestEmit_FirstClassSettings_WinsOverOverlay(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{"model": "old-model", "outputStyle": "from-overlay", "customKey": "preserved"}` + "\n"
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Settings: &config.ClaudeSettings{Model: "claude-opus-4-7"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["model"] != "claude-opus-4-7" {
		t.Errorf("config should override overlay model: %v", parsed["model"])
	}
	if parsed["outputStyle"] != "from-overlay" {
		t.Errorf("overlay value should survive when config silent: %v", parsed["outputStyle"])
	}
	if parsed["customKey"] != "preserved" {
		t.Errorf("unknown overlay key should pass through: %v", parsed["customKey"])
	}
}

func TestEmit_FirstClassSettings_ConfigOnly_NoOverlayNoHooks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Settings: &config.ClaudeSettings{Model: "claude-opus-4-7"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatalf("config-only settings should still produce file: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["model"] != "claude-opus-4-7" {
		t.Errorf("model not written from config: %v", parsed)
	}
	if _, ok := parsed["hooks"]; ok {
		t.Errorf("hooks key should not appear without hook specs: %v", parsed)
	}
}

func TestEmit_FirstClassSettings_WithHooks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Settings: &config.ClaudeSettings{
				Permissions: &config.ClaudePermissions{Allow: []string{"Read(*)"}},
			}},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{
			"event": "PostToolUse", "matcher": "Edit", "command": "fmt",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["permissions"]; !ok {
		t.Errorf("permissions missing: %s", raw)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("hooks missing: %s", raw)
	}
}

func TestEmit_FirstClassSettings_NilSettingsLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Dir: ".claude"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json should not be written when nothing configured: %v", err)
	}
}

func TestBuildConfigSettings_EmptyOnNilConfig(t *testing.T) {
	if got := buildConfigSettings(nil); len(got) != 0 {
		t.Errorf("nil config should yield empty map, got %v", got)
	}
	if got := buildConfigSettings(&config.Config{}); len(got) != 0 {
		t.Errorf("no outputs should yield empty map, got %v", got)
	}
}

func TestBuildConfigSettings_OmitsEmptyNested(t *testing.T) {
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"claude": {Settings: &config.ClaudeSettings{
				StatusLine:  &config.ClaudeStatusLine{},
				Permissions: &config.ClaudePermissions{},
			}},
		},
	}
	got := buildConfigSettings(cfg)
	if _, ok := got["statusLine"]; ok {
		t.Errorf("empty statusLine should be omitted: %v", got)
	}
	if _, ok := got["permissions"]; ok {
		t.Errorf("empty permissions should be omitted: %v", got)
	}
}
