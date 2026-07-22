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

func settingsEntry(meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: "defaults", Path: "settings/defaults.yaml", Meta: meta}
}

func TestBuildSpecSettings_MapsPermissionsAndModel(t *testing.T) {
	got := buildSpecSettings([]spec.Entry{settingsEntry(map[string]any{
		"model":       "claude-opus-4-8",
		"permissions": map[string]any{"allow": []any{"Bash(go test:*)"}, "deny": []any{"Bash(rm:*)"}},
	})})

	if got["model"] != "claude-opus-4-8" {
		t.Errorf("model = %v, want claude-opus-4-8", got["model"])
	}
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions missing or wrong type: %v", got["permissions"])
	}
	if allow, _ := perms["allow"].([]any); len(allow) != 1 || allow[0] != "Bash(go test:*)" {
		t.Errorf("allow = %v", perms["allow"])
	}
	if _, hasAsk := perms["ask"]; hasAsk {
		t.Errorf("ask must be omitted when empty, got %v", perms["ask"])
	}
}

func TestBuildSpecSettings_MergesMultipleSpecsDeduped(t *testing.T) {
	got := buildSpecSettings([]spec.Entry{
		settingsEntry(map[string]any{"permissions": map[string]any{"allow": []any{"A", "B"}}}),
		settingsEntry(map[string]any{"permissions": map[string]any{"allow": []any{"B", "C"}}, "model": "m"}),
	})
	allow, _ := got["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 3 {
		t.Fatalf("expected deduped allow of 3, got %v", allow)
	}
	want := []any{"A", "B", "C"}
	for i := range want {
		if allow[i] != want[i] {
			t.Errorf("allow[%d] = %v, want %v", i, allow[i], want[i])
		}
	}
	if got["model"] != "m" {
		t.Errorf("model = %v, want m", got["model"])
	}
}

func TestBuildSpecSettings_EmptyWhenNoFields(t *testing.T) {
	if got := buildSpecSettings(nil); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
	if got := buildSpecSettings([]spec.Entry{settingsEntry(map[string]any{"unrelated": "x"})}); len(got) != 0 {
		t.Errorf("expected empty map for unmapped fields, got %v", got)
	}
}

// A settings spec alone (no hooks, no config) still writes settings.json.
func TestEmit_SettingsSpecWritesSettingsJSON(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{settingsEntry(map[string]any{
		"model":       "claude-opus-4-8",
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
	})})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	if doc["model"] != "claude-opus-4-8" {
		t.Errorf("model = %v", doc["model"])
	}
	if _, ok := doc["permissions"]; !ok {
		t.Errorf("permissions missing: %s", raw)
	}
}

// Claude-specific config (`outputs.claude.settings`) overrides the generic
// settings spec for scalars like model, since it is the more specific source.
func TestEmit_ConfigSettingsOverrideSpec(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{settingsEntry(map[string]any{"model": "spec-model"})})
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {Settings: &config.ClaudeSettings{Model: "config-model"}},
	}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := readSettings(t, dir)["model"]; got != "config-model" {
		t.Errorf("config must win: model = %v, want config-model", got)
	}
}

// permissions are additive across layers: spec allow + config deny must both
// survive, not clobber each other (a wholesale key replace would drop one).
func TestEmit_SpecAndConfigPermissionsUnion(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{settingsEntry(map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(go test:*)"}},
	})})
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {Settings: &config.ClaudeSettings{Permissions: &config.ClaudePermissions{Deny: []string{"Bash(rm:*)"}}}},
	}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	perms := readSettings(t, dir)["permissions"].(map[string]any)
	if allow, _ := perms["allow"].([]any); len(allow) != 1 || allow[0] != "Bash(go test:*)" {
		t.Errorf("spec allow lost: %v", perms["allow"])
	}
	if deny, _ := perms["deny"].([]any); len(deny) != 1 || deny[0] != "Bash(rm:*)" {
		t.Errorf("config deny lost: %v", perms["deny"])
	}
}

// An imported overlay permissions block (the base layer) must survive a
// settings spec touching only a different sub-list (#432 review): the spec's
// deny must not erase the overlay's allow.
func TestEmit_OverlayPermissionsSurviveSpec(t *testing.T) {
	dir := testutil.TempCwd(t)
	// Overlay carries a list rule AND a sibling key the adapter does not
	// model (defaultMode). Both must survive a spec that touches only deny.
	overlay := `{"permissions":{"defaultMode":"acceptEdits","allow":["Bash(overlay:*)"]}}`
	overlayPath := filepath.Join(dir, ".agnostic-ai", "overlays", "claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{settingsEntry(map[string]any{
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
	})})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	perms := readSettings(t, dir)["permissions"].(map[string]any)
	if allow, _ := perms["allow"].([]any); len(allow) != 1 || allow[0] != "Bash(overlay:*)" {
		t.Errorf("overlay allow lost: %v", perms["allow"])
	}
	if deny, _ := perms["deny"].([]any); len(deny) != 1 || deny[0] != "Bash(rm:*)" {
		t.Errorf("spec deny missing: %v", perms["deny"])
	}
	if perms["defaultMode"] != "acceptEdits" {
		t.Errorf("overlay sibling key defaultMode dropped: %v", perms["defaultMode"])
	}
}

func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	return doc
}
