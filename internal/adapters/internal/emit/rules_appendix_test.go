package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func ruleBundle() spec.Bundle {
	return spec.Bundle{Rules: []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule one body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule two body"},
	}}
}

func TestRenderRulesAppendix_EmitsEachRuleBody(t *testing.T) {
	got := RenderRulesAppendix(ruleBundle())
	for _, want := range []string{
		RulesStartMarker, RulesEndMarker,
		"## Rules", "### r1", "rule one body", "### r2", "rule two body",
		"<!-- source: rules/r1.md -->",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("appendix missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRulesAppendix_EmptyWhenNoRules(t *testing.T) {
	if got := RenderRulesAppendix(spec.Bundle{}); got != "" {
		t.Errorf("want empty appendix for ruleless bundle, got %q", got)
	}
}

func TestAppendRulesAppendix_DoesNotStack(t *testing.T) {
	body := "# Pointer body\n"
	app := RenderRulesAppendix(ruleBundle())
	once := AppendRulesAppendix(body, app)
	twice := AppendRulesAppendix(once, app)
	if once != twice {
		t.Errorf("re-appending stacked the block:\nonce:\n%s\ntwice:\n%s", once, twice)
	}
	if n := strings.Count(twice, RulesStartMarker); n != 1 {
		t.Errorf("want exactly one rules block, got %d", n)
	}
}

func TestStripRulesAppendix_RoundTrips(t *testing.T) {
	body := "# Pointer body\n"
	withRules := AppendRulesAppendix(body, RenderRulesAppendix(ruleBundle()))
	if got := StripRulesAppendix(withRules); got != body {
		t.Errorf("strip did not restore body.\nwant %q\ngot  %q", body, got)
	}
}

func TestStripGeneratedAppendices_RemovesRulesAndOverview(t *testing.T) {
	body := "# Pointer body\n"
	withRules := AppendRulesAppendix(body, RenderRulesAppendix(ruleBundle()))
	withBoth := AppendTargetOverview(withRules, RenderTargetOverview([]TargetArtifacts{
		{Target: "codex", Artifacts: []NativeArtifact{{Label: "Agents", Location: ".codex/agents/"}}},
	}))
	if got := StripGeneratedAppendices(withBoth); got != body {
		t.Errorf("strip did not restore canonical body.\nwant %q\ngot  %q", body, got)
	}
}

func importCfg(mode string) *config.Config {
	return &config.Config{Outputs: map[string]config.Output{"claude": {RulesMode: mode}}}
}

func TestImportsRulesIntoEntryPoint(t *testing.T) {
	if !ImportsRulesIntoEntryPoint(importCfg("import"), "claude") {
		t.Error("claude with rules-mode: import should import rules into CLAUDE.md")
	}
	if ImportsRulesIntoEntryPoint(importCfg(""), "claude") {
		t.Error("default rules-mode must not import")
	}
	if ImportsRulesIntoEntryPoint(importCfg("import"), "codex") {
		t.Error("codex has no per-rule directory and must not import")
	}
	if ImportsRulesIntoEntryPoint(nil, "claude") {
		t.Error("nil config must not import")
	}
}

func TestImportsRulesIntoEntryPoint_LegacyRulesFileOverrides(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {RulesMode: "import", RulesFile: "CLAUDE.md"},
	}}
	if ImportsRulesIntoEntryPoint(cfg, "claude") {
		t.Error("legacy rules-file must override import mode")
	}
}

func TestRenderRulesImportAppendix_EmitsImportLines(t *testing.T) {
	got := RenderRulesImportAppendix(importCfg("import"), "claude", ruleBundle())
	for _, want := range []string{
		RulesStartMarker, RulesEndMarker, "## Rules",
		"@.claude/rules/r1.md", "@.claude/rules/r2.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("import appendix missing %q:\n%s", want, got)
		}
	}
	// Import mode references files, never inlines bodies.
	if strings.Contains(got, "rule one body") {
		t.Errorf("import appendix must not inline rule bodies:\n%s", got)
	}
}

func TestRenderRulesImportAppendix_HonorsRulesDirOverride(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{
		"claude": {RulesMode: "import", RulesDir: ".claude/conventions"},
	}}
	got := RenderRulesImportAppendix(cfg, "claude", ruleBundle())
	if !strings.Contains(got, "@.claude/conventions/r1.md") {
		t.Errorf("import appendix ignored rules-dir override:\n%s", got)
	}
}

func TestRenderRulesImportAppendix_EmptyWhenNoRules(t *testing.T) {
	if got := RenderRulesImportAppendix(importCfg("import"), "claude", spec.Bundle{}); got != "" {
		t.Errorf("want empty import appendix for ruleless bundle, got %q", got)
	}
}

func TestRenderRulesImportAppendix_RoundTrips(t *testing.T) {
	body := "# Pointer body\n"
	withImports := AppendRulesAppendix(body, RenderRulesImportAppendix(importCfg("import"), "claude", ruleBundle()))
	if got := StripRulesAppendix(withImports); got != body {
		t.Errorf("strip did not restore body.\nwant %q\ngot  %q", body, got)
	}
}

func TestInlinesRulesIntoEntryPoint(t *testing.T) {
	inline := []string{"codex", "amp", "warp", "zed", "gemini", "aider", "opencode"}
	for _, tgt := range inline {
		if !InlinesRulesIntoEntryPoint(tgt) {
			t.Errorf("%s should inline rules into its entry-point", tgt)
		}
	}
	// Targets with a native rules destination must not inline.
	for _, tgt := range []string{"claude", "cursor", "cline", "continue", "windsurf", "antigravity", "copilot"} {
		if InlinesRulesIntoEntryPoint(tgt) {
			t.Errorf("%s has a native rules destination and must not inline", tgt)
		}
	}
}
