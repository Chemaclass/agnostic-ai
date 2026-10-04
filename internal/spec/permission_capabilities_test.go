package spec

import (
	"slices"
	"testing"
)

func TestNeutralPermission_RoundTripsThroughPermissionRules(t *testing.T) {
	for rule, want := range map[string]string{
		"Read":            "read",
		"Read(src/**)":    "read(src/**)",
		"Edit(.env)":      "edit(.env)",
		"Write":           "write",
		"Bash(go test:*)": "shell(go test:*)",
		"mcp__github__x":  "mcp:github/x",
	} {
		got, ok := NeutralPermission(rule)
		if !ok || got != want {
			t.Errorf("NeutralPermission(%q) = %q, %v; want %q", rule, got, ok, want)
			continue
		}
		if back, problem := PermissionRules("allow", got); problem != "" || len(back) != 1 || back[0] != rule {
			t.Errorf("PermissionRules(%q) = %v, %q; want [%s]", got, back, problem, rule)
		}
	}
	for _, rule := range []string{"Write(.env)", "WebFetch(domain:go.dev)", "Read()", "Grep"} {
		if got, ok := NeutralPermission(rule); ok {
			t.Errorf("NeutralPermission(%q) = %q, want no capability", rule, got)
		}
	}
}

// NativePermissions expands capabilities in place and leaves the source
// entry, rules it cannot read, and Claude Code rules as written.
func TestNativePermissions_ExpandsCapabilitiesOnly(t *testing.T) {
	perms := map[string]any{
		"allow":        []any{"read", "web", "Bash(go test:*)"},
		"deny":         []any{"raed", "edit(.env)"},
		"default-mode": "plan",
	}
	e := Entry{Kind: KindSettings, Meta: map[string]any{"permissions": perms}}
	got := e.NativePermissions().Meta["permissions"].(map[string]any)
	if allow := got["allow"].([]any); !slices.Equal(allow, []any{"Read", "WebFetch", "WebSearch", "Bash(go test:*)"}) {
		t.Errorf("allow = %v", allow)
	}
	if deny := got["deny"].([]any); !slices.Equal(deny, []any{"raed", "Edit(.env)"}) {
		t.Errorf("deny = %v", deny)
	}
	if got["default-mode"] != "plan" {
		t.Errorf("default-mode = %v", got["default-mode"])
	}
	if allow := perms["allow"].([]any); allow[0] != "read" {
		t.Error("NativePermissions must not change the source entry")
	}
}
