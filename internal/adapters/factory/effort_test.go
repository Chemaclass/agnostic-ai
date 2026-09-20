package factory

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Factory writes its own `.factory/droids/<name>.md`, so a portable
// field it documents belongs in the file rather than in a coverage
// note. `mcpServers` narrows which servers a droid may reach, and
// losing it leaves the droid with the session's full tool surface.
func TestDroidMarkdown_EmitsPortableMCPScopeAndEffort(t *testing.T) {
	body, _ := droidMarkdown(spec.Entry{
		Kind: spec.KindAgent, Name: "architect", Body: "Design it.",
		Meta: map[string]any{
			"description": "Design work",
			"mcpServers":  []any{"slack"},
			"effort":      "high",
		},
	})
	for _, want := range []string{"mcpServers:", "slack", "reasoningEffort: high"} {
		if !strings.Contains(body, want) {
			t.Errorf("droid frontmatter missing %q:\n%s", want, body)
		}
	}
	// The portable spelling must not leak beside the native one.
	if strings.Contains(body, "effort: high\n") && !strings.Contains(body, "reasoningEffort: high") {
		t.Errorf("wrote the portable key instead of Factory's own:\n%s", body)
	}
}

// Factory's enum stops at high. Qoder and Claude Code document xhigh,
// max, and an integer budget, so those have no Factory spelling and
// must not be written into a file Droid CLI validates at load time.
func TestDroidReasoningEffort_RejectsValuesOutsideFactorysEnum(t *testing.T) {
	cases := map[string]bool{"low": true, "medium": true, "high": true, "xhigh": false, "max": false, "4": false}
	for value, want := range cases {
		got, ok := droidReasoningEffort(map[string]any{"effort": value})
		if got != value {
			t.Errorf("droidReasoningEffort(%q) value = %q", value, got)
		}
		if ok != want {
			t.Errorf("droidReasoningEffort(%q) accepted = %v, want %v", value, ok, want)
		}
	}
}

// A YAML integer budget is the shape the string form misses. yaml.v3
// hands it over as int, int64, or float64 depending on the scalar, and
// every one of them has to reach the note rather than read as absent.
func TestDroidReasoningEffort_CountsIntegerBudget(t *testing.T) {
	for _, value := range []any{8000, int64(8000), float64(8000)} {
		got, ok := droidReasoningEffort(map[string]any{"effort": value})
		if got != "8000" || ok {
			t.Errorf("droidReasoningEffort(%T %v) = (%q, %v), want (\"8000\", false)", value, value, got, ok)
		}
	}
}

// A native reasoningEffort the author wrote wins over the portable one.
func TestDroidReasoningEffort_NativeKeyWins(t *testing.T) {
	got, ok := droidReasoningEffort(map[string]any{"effort": "max", "reasoningEffort": "medium"})
	if got != "medium" || !ok {
		t.Errorf("got (%q, %v), want (\"medium\", true)", got, ok)
	}
}

// No effort anywhere means no key and no note.
func TestDroidReasoningEffort_AbsentIsQuiet(t *testing.T) {
	if got, ok := droidReasoningEffort(map[string]any{}); got != "" || ok {
		t.Errorf("got (%q, %v), want (\"\", false)", got, ok)
	}
}
