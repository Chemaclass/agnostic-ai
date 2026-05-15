package codex

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestAgentTOML_EscapesQuotesAndBackslashes(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "tricky",
		Body: `Body with "quotes" and a \ backslash and a """ run.`,
		Meta: map[string]any{"description": `Says "hi" with a \ slash`},
	}
	got := agentTOML(a)
	for _, want := range []string{
		`description = "Says \"hi\" with a \\ slash"`,
		`Body with \"quotes\" and a \\ backslash and a \"\"\" run.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestAgentTOML_BodyFallsBackToDescription(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "minimal",
		Meta: map[string]any{"description": "describe me"},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `description = "describe me"`) {
		t.Errorf("missing description:\n%s", got)
	}
	if !strings.Contains(got, "describe me\n\"\"\"") {
		t.Errorf("body should fall back to description when empty:\n%s", got)
	}
}

func TestAgentTOML_DescriptionFallsBackToName(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "explorer",
		Body: "Find things.",
	}
	got := agentTOML(a)
	if !strings.Contains(got, `description = "explorer"`) {
		t.Errorf("description should fall back to name:\n%s", got)
	}
}

func TestAgentTOML_XCodexOverridesTopLevel(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"model": "gpt-5",
			"x-codex": map[string]any{
				"model": "gpt-5.4",
			},
		},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `model = "gpt-5.4"`) {
		t.Errorf("x-codex.model should win over top-level model:\n%s", got)
	}
	if strings.Contains(got, `model = "gpt-5"`+"\n") {
		t.Errorf("top-level model should not survive once x-codex.model is set:\n%s", got)
	}
}

func TestAgentTOML_ToolsFirstClass(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "scoped",
		Body: "Limited tool set.",
		Meta: map[string]any{
			"tools": []any{"bash", "edit", "read"},
		},
	}
	got := agentTOML(a)
	if !strings.Contains(got, `tools = ["bash", "edit", "read"]`) {
		t.Errorf("missing tools array:\n%s", got)
	}
}

func TestAgentTOML_XCodexToolsOverridesTopLevel(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "scoped",
		Body: "x",
		Meta: map[string]any{
			"tools": []any{"bash"},
			"x-codex": map[string]any{
				"tools": []any{"edit", "read"},
			},
		},
	}
	got := agentTOML(a)
	// Only one tools line, with x-codex content winning.
	if !strings.Contains(got, `tools = ["edit", "read"]`) {
		t.Errorf("x-codex.tools should win:\n%s", got)
	}
	if strings.Count(got, "tools = ") != 1 {
		t.Errorf("tools should emit exactly once:\n%s", got)
	}
}

func TestAgentTOML_NicknameCandidatesIgnoresNonStringSlice(t *testing.T) {
	a := spec.Entry{
		Kind: spec.KindAgent,
		Name: "ag",
		Body: "x",
		Meta: map[string]any{
			"x-codex": map[string]any{
				"nickname_candidates": []any{"Atlas", 42}, // mixed types: skip
			},
		},
	}
	got := agentTOML(a)
	if strings.Contains(got, "nickname_candidates") {
		t.Errorf("non-string slice should be skipped:\n%s", got)
	}
}
