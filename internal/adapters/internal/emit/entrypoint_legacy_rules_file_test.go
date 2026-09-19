package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// The legacy rules-file skip exists for one reason: two writers on one
// path. Anything wider drops a target's entry-point file for no gain,
// and on claude that hands Claude Code the AGENTS.md fallback.
func TestLegacyRulesFileOwnsEntryPoint(t *testing.T) {
	cases := []struct {
		name   string
		target string
		out    config.Output
		want   bool
	}{
		{name: "unset", target: "claude", out: config.Output{}, want: false},
		{name: "own entry point", target: "claude", out: config.Output{RulesFile: "CLAUDE.md"}, want: true},
		{name: "own entry point, dot-slash spelling", target: "claude", out: config.Output{RulesFile: "./CLAUDE.md"}, want: true},
		{name: "off entry point", target: "claude", out: config.Output{RulesFile: ".claude/RULES.md"}, want: false},
		{name: "codex own entry point", target: "codex", out: config.Output{RulesFile: "AGENTS.md"}, want: true},
		{name: "codex off entry point", target: "codex", out: config.Output{RulesFile: "AGENTS-rules.md"}, want: false},
		{name: "follows a file override", target: "claude", out: config.Output{File: "docs/CLAUDE.md", RulesFile: "docs/CLAUDE.md"}, want: true},
		{name: "no entry-point convention", target: "cursor", out: config.Output{RulesFile: ".cursor/RULES.md"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Outputs: map[string]config.Output{tc.target: tc.out}}
			if got := LegacyRulesFileOwnsEntryPoint(cfg, tc.target); got != tc.want {
				t.Errorf("LegacyRulesFileOwnsEntryPoint(%q, %+v) = %v, want %v", tc.target, tc.out, got, tc.want)
			}
		})
	}
}

// The `@`-import is what makes a merged file outside `.claude/rules/`
// reachable at all. It must not fire for the documented
// `rules-file: CLAUDE.md` form, where the adapter owns the whole file.
func TestRenderLegacyRulesFileImportAppendix(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {RulesFile: ".claude/RULES.md"},
	}}
	got := RenderLegacyRulesFileImportAppendix(cfg, "claude")
	if got == "" {
		t.Fatal("expected an import appendix for a rules-file off the entry point")
	}
	if !strings.Contains(got, "@.claude/RULES.md") {
		t.Errorf("expected the rules-file import line, got:\n%s", got)
	}
	if !strings.Contains(got, RulesStartMarker) || !strings.Contains(got, RulesEndMarker) {
		t.Errorf("appendix must be sentinel-marked so import strips it, got:\n%s", got)
	}

	own := &config.Config{Outputs: map[string]config.Output{"claude": {RulesFile: "CLAUDE.md"}}}
	if got := RenderLegacyRulesFileImportAppendix(own, "claude"); got != "" {
		t.Errorf("the adapter owns CLAUDE.md in that layout, got:\n%s", got)
	}

	// Only targets whose CLI auto-loads the entry point but not the
	// rules path get the line. Gemini inlines rule bodies instead.
	gem := &config.Config{Outputs: map[string]config.Output{"gemini": {RulesFile: "GEMINI-rules.md"}}}
	if got := RenderLegacyRulesFileImportAppendix(gem, "gemini"); got != "" {
		t.Errorf("gemini has no `@`-import route, got:\n%s", got)
	}
}
