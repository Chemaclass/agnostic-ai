package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestWithoutUnsupportedDelete_ClaudeNativePermissionsAreAdditive(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Path: "settings/defaults.yaml", Meta: map[string]any{
		"permissions": map[string]any{"allow": []any{"Delete"}},
		"x-claude":    map[string]any{"permissions": map[string]any{"deny": []any{"Bash"}}},
	}}})
	_, err := WithoutUnsupportedDelete(b, "claude", OnUnsupportedError)
	if err == nil || !strings.Contains(err.Error(), "permissions.allow capability delete") {
		t.Errorf("error = %v, want unsupported delete", err)
	}
}

func TestWithoutUnsupportedDelete_ClaudeWarnRemovesSyntheticTool(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Path: "settings/defaults.yaml", Meta: map[string]any{
		"permissions": map[string]any{"allow": []any{"Read", "Delete"}},
		"x-claude":    map[string]any{"permissions": map[string]any{"deny": []any{"Bash"}}},
	}}})
	got, err := WithoutUnsupportedDelete(b, "claude", OnUnsupportedSilent)
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	permissions, _ := got.Settings[0].Meta["permissions"].(map[string]any)
	names := StringSlice(permissions["allow"])
	if len(names) != 1 || names[0] != "Read" {
		t.Errorf("allow = %v, want Read", names)
	}
}

func TestWithoutUnsupportedDelete_AuthoritativeNativeSettingsReplacePortable(t *testing.T) {
	for _, tc := range []struct{ target, key string }{{"kilo", "permission"}, {"windsurf", "permissions"}} {
		t.Run(tc.target, func(t *testing.T) {
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{
				"permissions":    map[string]any{"allow": []any{"Delete"}},
				"x-" + tc.target: map[string]any{tc.key: map[string]any{"deny": []any{"Bash"}}},
			}}})
			if _, err := WithoutUnsupportedDelete(b, tc.target, OnUnsupportedError); err != nil {
				t.Errorf("unused delete failed: %v", err)
			}
		})
	}
}
