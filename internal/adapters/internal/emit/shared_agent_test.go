package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSharedAgentMarkdown_RendersPortableFields(t *testing.T) {
	agent := spec.Entry{
		Name: "reviewer",
		Meta: map[string]any{
			"description": "Reviews code changes.",
			"model":       "anthropic/claude-sonnet-4",
			"tools":       []any{"Read", "Grep"},
			"x-goose":     map[string]any{"temperature": 0.2},
		},
		Body: "\nReview the diff.\n",
	}

	got := SharedAgentMarkdown(agent, "openhands")
	for _, want := range []string{
		"name: reviewer",
		"description: Reviews code changes.",
		"model: anthropic/claude-sonnet-4",
		"Review the diff.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, absent := range []string{"tools:", "temperature:", "x-goose:"} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected %q in:\n%s", absent, got)
		}
	}
}

func TestSharedAgentMarkdown_DescriptionFallsBackToName(t *testing.T) {
	got := SharedAgentMarkdown(spec.Entry{Name: "reviewer"}, "goose")
	if !strings.Contains(got, "description: reviewer") {
		t.Errorf("missing description fallback in:\n%s", got)
	}
}

func TestSharedAgentMarkdown_PreservesExplicitTargetMetadata(t *testing.T) {
	agent := spec.Entry{
		Name: "reviewer",
		Meta: map[string]any{
			"x-openhands": map[string]any{"tools": []any{"file_editor", "terminal"}},
		},
	}
	got := SharedAgentMarkdown(agent, "openhands")
	if !strings.Contains(got, "tools:") || !strings.Contains(got, "file_editor") {
		t.Errorf("explicit OpenHands tools missing from:\n%s", got)
	}
}

func TestSharedAgentToolsDropped_RespectsTargetOverrideAndDelete(t *testing.T) {
	for _, value := range []any{[]any{"terminal"}, nil} {
		agent := spec.Entry{Meta: map[string]any{
			"tools":       []any{"Read"},
			"x-openhands": map[string]any{"tools": value},
		}}
		if SharedAgentToolsDropped(agent, "openhands") {
			t.Errorf("explicit x-openhands.tools value %v should suppress the drop note", value)
		}
	}

	agent := spec.Entry{Meta: map[string]any{"tools": []any{"Read"}}}
	if !SharedAgentToolsDropped(agent, "openhands") {
		t.Error("generic tools without a target override should report a drop")
	}
}
