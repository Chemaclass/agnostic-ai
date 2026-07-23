package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestOutputResolvers_FallbackWhenUnset(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cases := []struct {
		name string
		got  string
	}{
		{"OutputFile", OutputFile(cfg, "x", "default.md")},
		{"OutputDir", OutputDir(cfg, "x", ".x")},
		{"OutputRulesDir", OutputRulesDir(cfg, "x", ".x/rules")},
		{"OutputRulesFile", OutputRulesFile(cfg, "x", "X.md")},
		{"OutputMCPFile", OutputMCPFile(cfg, "x", ".x/mcp.json")},
	}
	for _, c := range cases {
		if c.got == "" {
			t.Errorf("%s: expected fallback, got empty", c.name)
		}
	}
}

func TestOutputResolvers_OverrideWins(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"x": {
				File:      "out.md",
				Dir:       "out-dir",
				RulesFile: "out-rules.md",
				RulesDir:  "out-rules",
				MCPFile:   "out-mcp.json",
			},
		},
	}
	if got := OutputFile(cfg, "x", "default.md"); got != "out.md" {
		t.Errorf("OutputFile = %q", got)
	}
	if got := OutputDir(cfg, "x", "default"); got != "out-dir" {
		t.Errorf("OutputDir = %q", got)
	}
	if got := OutputRulesDir(cfg, "x", "default"); got != "out-rules" {
		t.Errorf("OutputRulesDir = %q", got)
	}
	if got := OutputRulesFile(cfg, "x", "default"); got != "out-rules.md" {
		t.Errorf("OutputRulesFile = %q", got)
	}
	if got := OutputMCPFile(cfg, "x", "default"); got != "out-mcp.json" {
		t.Errorf("OutputMCPFile = %q", got)
	}
}

func TestOutputResolvers_EmptyOverrideUsesFallback(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Outputs: map[string]config.Output{"x": {}}}
	if got := OutputFile(cfg, "x", "fallback"); got != "fallback" {
		t.Errorf("expected fallback when override empty, got %q", got)
	}
}
