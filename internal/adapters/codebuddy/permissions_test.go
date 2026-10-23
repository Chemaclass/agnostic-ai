package codebuddy

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPermissionMCPRulesFollowNativeSegmentGrammar(t *testing.T) {
	for _, tc := range []struct {
		list, rule string
		accepted   bool
	}{
		{"allow", "mcp__github", true},
		{"allow", "mcp__github__*", true},
		{"allow", "mcp__github__get_issue", true},
		{"deny", "mcp__web-search", true},
		{"ask", "mcp__web_search__get_issue", true},
		{"ask", "mcp__*", true},
		{"deny", "mcp__*", true},
		{"allow", "mcp__*", false},
		{"deny", "mcp__git*", false},
		{"deny", "mcp__github__get*", false},
		{"allow", "mcp__github__get?issue", false},
		{"allow", "mcp__", false},
		{"allow", "mcp__github__", false},
		{"allow", "mcp____get_issue", false},
		{"allow", "mcp__github____get_issue", false},
		{"allow", "mcp__*__get_issue", false},
		{"allow", "mcp__github__get_issue(arg)", false},
	} {
		t.Run(tc.list+"/"+tc.rule, func(t *testing.T) {
			got, ok := New().TranslatePermission(tc.list, tc.rule)
			if ok != tc.accepted {
				t.Fatalf("accepted=%v, want %v", ok, tc.accepted)
			}
			if ok && !slices.Equal(got, []string{tc.rule}) {
				t.Errorf("mapping changed native rule: %v", got)
			}
		})
	}
}

func TestPermissionMCPRulesStaySeparateFromAgentExposure(t *testing.T) {
	for _, rule := range []string{"mcp__github", "mcp__github__*", "mcp__*", "mcp____get_issue", "mcp__github____get_issue"} {
		if _, ok := New().TranslateAgentCapability(rule); ok {
			t.Errorf("agent accepted permission-only or malformed rule %q", rule)
		}
	}
	if names, ok := New().TranslateAgentCapability("mcp__github__get_issue"); !ok || !slices.Equal(names, []string{"mcp__github__get_issue"}) {
		t.Errorf("exact agent tool rejected or changed: %v", names)
	}
}

func TestEmit_PreservesVerifiedPermissionMCPRules(t *testing.T) {
	testutil.TempCwd(t)
	permissions := map[string]any{
		"allow": []any{"mcp__github", "mcp__github__*", "mcp__github__get_issue", "Bash(git diff *)"},
		"ask":   []any{"mcp__*"},
		"deny":  []any{"mcp__*", "mcp__github__delete_issue"},
	}
	b := spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": permissions}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got struct{ Permissions map[string][]string }
	if err := json.Unmarshal([]byte(read(t, ".codebuddy/settings.json")), &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range permissions {
		if !slices.Equal(got.Permissions[key], list(want)) {
			t.Errorf("%s=%v, want %v", key, got.Permissions[key], want)
		}
	}
}
