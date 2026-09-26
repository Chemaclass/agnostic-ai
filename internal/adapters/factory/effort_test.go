package factory

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Factory writes its own `.factory/droids/<name>.md`, so a portable
// field it documents belongs in the file rather than in a coverage
// note. `mcpServers` narrows which servers a droid may reach, and
// losing it leaves the droid with the session's full tool surface.
func TestDroidMarkdown_EmitsPortableMCPScopeAndEffort(t *testing.T) {
	body, _, _ := droidMarkdown(spec.Entry{
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

// Factory documents `mcpServers: []` as excluding every server, "even
// globally configured ones", while an absent key inherits them all. An
// empty list must therefore reach the droid file, or a droid meant to
// have no MCP access silently gets every server.
func TestDroidMarkdown_KeepsEmptyMCPServersList(t *testing.T) {
	body, _, _ := droidMarkdown(spec.Entry{
		Kind: spec.KindAgent, Name: "offline", Body: "No MCP.",
		Meta: map[string]any{"description": "No MCP", "mcpServers": []any{}},
	})
	if !strings.Contains(body, "mcpServers: []\n") {
		t.Errorf("empty mcpServers must be written, got:\n%s", body)
	}

	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "offline", Meta: map[string]any{"mcpServers": []any{}}}
	if err := emit.ReportUnsupported(caps, spec.Bundle{Agents: []spec.Entry{agent}}, ""); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "mcpServers") {
		t.Errorf("a written list must not raise a drop note, got: %s", buf.String())
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

// A per-target `effort` map collapses in emit before Factory ever sees
// it, so the note path reads the same scalar it always did. This guards
// the seam: the collapser is generic, and nothing in this adapter knows
// the map form exists (#968).
func TestDroidReasoningEffort_ReadsThroughThePerTargetMap(t *testing.T) {
	meta := map[string]any{
		"effort": map[string]any{"factory": "max", "default": "high"},
	}
	got, ok := droidReasoningEffort(emit.ResolveMeta(meta, target))
	if got != "max" || ok {
		t.Errorf("got (%q, %v), want (\"max\", false)", got, ok)
	}
	// The same spec on a target the map does not name falls back to
	// `default`, which Factory does accept.
	fallback := map[string]any{
		"effort": map[string]any{"claude": "xhigh", "default": "high"},
	}
	if got, ok := droidReasoningEffort(emit.ResolveMeta(fallback, target)); got != "high" || !ok {
		t.Errorf("got (%q, %v), want (\"high\", true)", got, ok)
	}
}
