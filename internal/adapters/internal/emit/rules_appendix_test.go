package emit

import (
	"strings"
	"testing"

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

func TestInlinesRulesIntoEntryPoint(t *testing.T) {
	inline := []string{"codex", "amp", "warp", "gemini", "aider", "opencode"}
	for _, tgt := range inline {
		if !InlinesRulesIntoEntryPoint(tgt) {
			t.Errorf("%s should inline rules into its entry-point", tgt)
		}
	}
	// Targets with a native rules destination must not inline.
	for _, tgt := range []string{"claude", "cursor", "cline", "continue", "windsurf", "antigravity", "zed", "copilot"} {
		if InlinesRulesIntoEntryPoint(tgt) {
			t.Errorf("%s has a native rules destination and must not inline", tgt)
		}
	}
}
