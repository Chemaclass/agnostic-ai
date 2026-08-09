package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestLintEmptySpecs_FlagsEmptyBodyAndNoDescription(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "empty", Path: "rules/empty.md", Body: "", Meta: map[string]any{}},
		{Kind: spec.KindAgent, Name: "ok", Path: "agents/ok.md", Body: "Do something.", Meta: map[string]any{}},
		{Kind: spec.KindRule, Name: "desc-only", Path: "rules/desc.md", Body: "", Meta: map[string]any{"description": "has a description"}},
	}
	findings := lintEmptySpecs(entries)
	if len(findings) != 1 {
		t.Fatalf("expected 1 empty-spec finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT001" {
		t.Errorf("expected code LINT001, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
	if f.Path != "rules/empty.md" {
		t.Errorf("expected path rules/empty.md, got %s", f.Path)
	}
}

func TestLintUnterminatedFrontmatter_FlagsMissingClosingDelimiter(t *testing.T) {
	entries := []spec.Entry{
		// Closing `---` omitted, so splitFrontmatter yields empty meta and
		// hands the raw YAML back as the body.
		{Kind: spec.KindAgent, Name: "broken", Path: "agents/broken.md",
			Body: "---\nname: broken\ndescription: Reviews code.\n", Meta: map[string]any{}},
		// Parsed frontmatter: meta is populated and the delimiters are gone.
		{Kind: spec.KindAgent, Name: "ok", Path: "agents/ok.md",
			Body: "Do something.", Meta: map[string]any{"description": "fine"}},
		// Body-only spec with no frontmatter at all stays clean.
		{Kind: spec.KindRule, Name: "prose", Path: "rules/prose.md",
			Body: "Prefer short functions.", Meta: map[string]any{}},
		// A horizontal rule after parsed frontmatter is not a broken block.
		{Kind: spec.KindRule, Name: "hr", Path: "rules/hr.md",
			Body: "---\nstill fine", Meta: map[string]any{"description": "fine"}},
	}
	findings := lintUnterminatedFrontmatter(entries)
	if len(findings) != 1 {
		t.Fatalf("expected 1 unterminated-frontmatter finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT006" {
		t.Errorf("expected code LINT006, got %s", f.Code)
	}
	if f.Severity != lintError {
		t.Errorf("expected error severity, got %s", f.Severity)
	}
	if f.Path != "agents/broken.md" {
		t.Errorf("expected path agents/broken.md, got %s", f.Path)
	}
}

func TestLintDuplicateNames_FlagsSameKindAndName(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "style", Path: "rules/style.md"},
		{Kind: spec.KindAgent, Name: "style", Path: "agents/style.md"},         // different kind — OK
		{Kind: spec.KindRule, Name: "style", Path: "rules/overrides/style.md"}, // duplicate
	}
	findings := lintDuplicateNames(entries)
	if len(findings) != 1 {
		t.Fatalf("expected 1 duplicate-name finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT003" {
		t.Errorf("expected code LINT003, got %s", f.Code)
	}
	if f.Severity != lintError {
		t.Errorf("expected error severity, got %s", f.Severity)
	}
}

func TestLintDeadSpecs_FlagsKindUnsupportedByAllTargets(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h", Path: "hooks/h.yaml"},
	}
	// copilot does not support hooks
	findings := lintDeadSpecs(entries, []string{"copilot"})
	if len(findings) != 1 {
		t.Fatalf("expected 1 dead-spec finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT004" {
		t.Errorf("expected code LINT004, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
}

func TestLintDeadSpecs_NoFindingWhenTargetSupports(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h", Path: "hooks/h.yaml"},
	}
	// claude supports hooks
	if got := lintDeadSpecs(entries, []string{"claude"}); len(got) != 0 {
		t.Errorf("expected 0 findings when target supports kind, got %d", len(got))
	}
}

func TestLintHookMatcherMisuse_FlagsMatcherOnNonToolEvent(t *testing.T) {
	hooks := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "bad", Path: "hooks/bad.yaml",
			Meta: map[string]any{"event": "SessionEnd", "matcher": "Bash"},
		},
		{
			Kind: spec.KindHook, Name: "ok", Path: "hooks/ok.yaml",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit"},
		},
		{
			Kind: spec.KindHook, Name: "no-matcher", Path: "hooks/nm.yaml",
			Meta: map[string]any{"event": "SessionEnd"},
		},
	}
	findings := lintHookMatcherMisuse(hooks)
	if len(findings) != 1 {
		t.Fatalf("expected 1 matcher-misuse finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT005" {
		t.Errorf("expected code LINT005, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
	if f.Path != "hooks/bad.yaml" {
		t.Errorf("expected path hooks/bad.yaml, got %s", f.Path)
	}
}

func TestLintHookMatcherMisuse_NoFindingsOnCleanHooks(t *testing.T) {
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "a", Path: "hooks/a.yaml", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash"}},
		{Kind: spec.KindHook, Name: "b", Path: "hooks/b.yaml", Meta: map[string]any{"event": "AfterTool", "matcher": "shell"}},
		{Kind: spec.KindHook, Name: "c", Path: "hooks/c.yaml", Meta: map[string]any{"event": "SessionStart"}},
	}
	if got := lintHookMatcherMisuse(hooks); len(got) != 0 {
		t.Errorf("expected 0 findings on clean hooks, got %d", len(got))
	}
}
