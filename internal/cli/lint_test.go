package cli

import (
	"strings"
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
	// aider does not support hooks (copilot gained native hooks
	// support in #629, so it can no longer stand in for a hook-less
	// target here)
	findings := lintDeadSpecs(entries, []string{"aider"})
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

// cursor.com/docs/hooks' own "Available matchers by hook" table lists
// these five alongside the tool and shell events already covered:
// subagentStart/subagentStop filter by subagent type, beforeSubmitPrompt
// matches UserPromptSubmit, stop matches Stop, afterAgentResponse
// matches AgentResponse, and afterAgentThought matches AgentThought
// (verified 2026-09-11). Flagging any of them tells a Cursor user to
// delete a filter Cursor honors (#734).
func TestLintHookMatcherMisuse_CursorMatcherEventsStayClean(t *testing.T) {
	cases := map[string]string{
		"subagentStart":      "explore",
		"subagentStop":       "explore",
		"beforeSubmitPrompt": "UserPromptSubmit",
		"stop":               "Stop",
		"afterAgentResponse": "AgentResponse",
		"afterAgentThought":  "AgentThought",
	}
	for event, matcher := range cases {
		t.Run(event, func(t *testing.T) {
			hooks := []spec.Entry{
				{
					Kind: spec.KindHook, Name: "h", Path: "hooks/h.yaml",
					Meta: map[string]any{"event": event, "matcher": matcher},
				},
			}
			if got := lintHookMatcherMisuse(hooks); len(got) != 0 {
				t.Errorf("expected no finding for %s, got %+v", event, got)
			}
		})
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

func TestLintMCPMissingRequiredField_FlagsEveryTransport(t *testing.T) {
	mcps := []spec.Entry{
		// stdio is the inferred default, so an entry with no `type` at
		// all still needs a command.
		{Kind: spec.KindMCP, Name: "implicit-stdio", Path: "mcps/implicit.yaml",
			Meta: map[string]any{"args": []any{"--flag"}}},
		{Kind: spec.KindMCP, Name: "explicit-stdio", Path: "mcps/explicit.yaml",
			Meta: map[string]any{"type": "stdio"}},
		{Kind: spec.KindMCP, Name: "http", Path: "mcps/http.yaml",
			Meta: map[string]any{"type": "http", "headers": map[string]any{"A": "b"}}},
		{Kind: spec.KindMCP, Name: "sse", Path: "mcps/sse.yaml",
			Meta: map[string]any{"type": "sse"}},
		{Kind: spec.KindMCP, Name: "ws", Path: "mcps/ws.yaml",
			Meta: map[string]any{"type": "ws"}},
	}
	findings := lintMCPMissingRequiredField(mcps)
	if len(findings) != len(mcps) {
		t.Fatalf("expected %d findings, got %d: %+v", len(mcps), len(findings), findings)
	}
	for _, f := range findings {
		if f.Code != "LINT008" {
			t.Errorf("expected code LINT008, got %s", f.Code)
		}
		if f.Severity != lintError {
			t.Errorf("%s: expected error severity, got %s", f.Path, f.Severity)
		}
	}
	if want := "command"; !strings.Contains(findings[0].Message, want) {
		t.Errorf("stdio message should name %q, got %q", want, findings[0].Message)
	}
	if want := "url"; !strings.Contains(findings[2].Message, want) {
		t.Errorf("remote message should name %q, got %q", want, findings[2].Message)
	}
}

func TestLintMCPMissingRequiredField_NoFindingsOnCompleteEntries(t *testing.T) {
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "stdio", Path: "mcps/stdio.yaml",
			Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindMCP, Name: "remote", Path: "mcps/remote.yaml",
			Meta: map[string]any{"type": "sse", "url": "https://example.com/sse"}},
		// An unmapped transport carries no required field this rule can
		// name, so it stays clean here rather than guess at one.
		{Kind: spec.KindMCP, Name: "odd", Path: "mcps/odd.yaml",
			Meta: map[string]any{"type": "carrier-pigeon"}},
		// Every MCP builder drops a nameless entry before it reads the
		// transport, so there is no missing field to name.
		{Kind: spec.KindMCP, Name: "", Path: "mcps/nameless.yaml",
			Meta: map[string]any{"type": "stdio"}},
	}
	if got := lintMCPMissingRequiredField(mcps); len(got) != 0 {
		t.Errorf("expected 0 findings, got %d: %+v", len(got), got)
	}
}

// The rule has to reach `lint` through collectLintFindings, not just
// exist: the LSP shares that collector, and a rule wired up anywhere
// else would be missing from the editor.
func TestCollectLintFindings_IncludesMCPMissingRequiredField(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "broken", Path: "mcps/broken.yaml",
			Meta: map[string]any{"type": "stdio", "description": "no command"}},
	})
	var found bool
	for _, f := range collectLintFindings([]string{"claude"}, b) {
		if f.Code == "LINT008" {
			found = true
		}
	}
	if !found {
		t.Error("expected collectLintFindings to report LINT008")
	}
}
