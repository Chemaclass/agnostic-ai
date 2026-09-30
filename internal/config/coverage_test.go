package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCoverageConfig_Validate(t *testing.T) {
	codex := CoverageTargets{"codex"}
	cases := []struct {
		name    string
		entries []CoverageAccept
		want    string
	}{
		{"field entry", []CoverageAccept{{Target: codex, Kind: "agents", Field: "tools", Reason: "known"}}, ""},
		{"via entry", []CoverageAccept{{Target: CoverageTargets{"aider"}, Kind: "skills", Via: "outputs.aider.rules-file", Reason: "known"}}, ""},
		{"surface entry", []CoverageAccept{{Target: CoverageTargets{"copilot"}, Kind: "hooks", Surface: "Copilot cloud agent", Reason: "known"}}, ""},
		{"no target", []CoverageAccept{{Kind: "agents", Field: "tools", Reason: "known"}}, "target is required"},
		{"empty target name", []CoverageAccept{{Target: CoverageTargets{"codex", " "}, Kind: "agents", Field: "tools", Reason: "known"}}, "target is required"},
		{"singular kind", []CoverageAccept{{Target: codex, Kind: "agent", Field: "tools", Reason: "known"}}, `unknown kind "agent"`},
		{"no reason", []CoverageAccept{{Target: codex, Kind: "agents", Field: "tools", Reason: " "}}, "reason is required"},
		{"no selector", []CoverageAccept{{Target: codex, Kind: "agents", Reason: "known"}}, "set one of field, via, or surface"},
		{"two selectors", []CoverageAccept{{Target: codex, Kind: "agents", Field: "tools", Via: "x", Reason: "known"}}, "set one of field, via, or surface"},
		{"duplicate", []CoverageAccept{
			{Target: CoverageTargets{"codex", "gemini"}, Kind: "agents", Field: "tools", Reason: "a"},
			{Target: CoverageTargets{"gemini"}, Kind: "agents", Field: "tools", Reason: "b"},
		}, "coverage.accept[1]: duplicates coverage.accept[0] for gemini"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CoverageConfig{Accept: tc.entries}.Validate("agnostic-ai.yaml")
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

func TestCoverageTargets_TakesOneNameOrAList(t *testing.T) {
	var got struct {
		One  CoverageTargets `yaml:"one"`
		Many CoverageTargets `yaml:"many"`
	}
	if err := yaml.Unmarshal([]byte("one: codex\nmany: [codex, gemini]\n"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.One) != 1 || got.One[0] != "codex" {
		t.Errorf("one name decoded as %v", got.One)
	}
	if len(got.Many) != 2 || got.Many[1] != "gemini" {
		t.Errorf("a list decoded as %v", got.Many)
	}
	if err := yaml.Unmarshal([]byte("one: {a: b}\n"), &got); err == nil {
		t.Error("a map target should fail to decode")
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
