package config

import (
	"strings"
	"testing"
)

func TestCoverageConfig_Validate(t *testing.T) {
	cases := []struct {
		name  string
		entry CoverageAccept
		want  string
	}{
		{"valid", CoverageAccept{Target: "codex", Kind: "agents", Field: "tools", Reason: "known"}, ""},
		{"no target", CoverageAccept{Kind: "agents", Reason: "known"}, "target is required"},
		{"singular kind", CoverageAccept{Target: "codex", Kind: "agent", Reason: "known"}, `unknown kind "agent"`},
		{"no reason", CoverageAccept{Target: "codex", Kind: "agents", Reason: " "}, "reason is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CoverageConfig{Accept: []CoverageAccept{tc.entry}}.Validate("agnostic-ai.yaml")
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCoverageAccept_SpecKind(t *testing.T) {
	if got := (CoverageAccept{Kind: "mcps"}).SpecKind(); got != "mcp" {
		t.Errorf("mcps maps to %q, want mcp", got)
	}
	if got := (CoverageAccept{Kind: "settings"}).SpecKind(); got != "settings" {
		t.Errorf("settings maps to %q, want settings", got)
	}
}
