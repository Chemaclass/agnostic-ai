package emit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func swapWarner(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := Warner
	Warner = buf
	t.Cleanup(func() { Warner = prev })
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)
	return buf
}

func TestReportUnsupported_FlushGroupsByKind(t *testing.T) {
	buf := swapWarner(t)
	b := spec.Bundle{
		Hooks: []spec.Entry{{Name: "h1"}, {Name: "h2"}, {Name: "h3"}, {Name: "h4"}},
	}
	for _, target := range []string{"aider", "cline", "cursor"} {
		caps := Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule}}
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("warnings must buffer until flush, got early output: %s", buf)
	}
	FlushCapabilityWarnings()
	got := buf.String()
	want := "  ! 4 hooks unsupported by aider, cline, cursor\n"
	if !strings.Contains(got, want) {
		t.Errorf("expected grouped line %q, got:\n%s", want, got)
	}
	if strings.Count(got, "hooks unsupported") != 1 {
		t.Errorf("expected exactly one hooks line after flush, got:\n%s", got)
	}
}

func TestReportUnsupported_DedupTargetKindWithinFlush(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	for i := 0; i < 3; i++ {
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	FlushCapabilityWarnings()
	got := buf.String()
	if strings.Count(got, "hook unsupported") != 1 {
		t.Errorf("expected single warning line for repeat reports of same (target, kind), got:\n%s", got)
	}
}

func TestReportUnsupported_PrintsSuppressionHintOnce(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{
		Hooks:    []spec.Entry{{Name: "h1"}},
		Commands: []spec.Entry{{Name: "c1"}},
	}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if strings.Count(buf.String(), "on-unsupported: silent") != 1 {
		t.Errorf("expected suppression hint exactly once per flush, got:\n%s", buf.String())
	}
}

func TestReportUnsupported_PluralizesCorrectly(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if !strings.Contains(buf.String(), "1 hook unsupported by aider") {
		t.Errorf("expected singular 'hook' for n=1, got:\n%s", buf.String())
	}
}

func TestReportUnsupported_SilentSkipsAll(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedSilent); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if buf.Len() != 0 {
		t.Errorf("silent mode must produce no output, got: %s", buf.String())
	}
}

func TestCapabilityWarningsDigest_StableAcrossOrder(t *testing.T) {
	swapWarner(t)
	bA := spec.Bundle{Hooks: []spec.Entry{{Name: "h"}, {Name: "h2"}}}
	for _, target := range []string{"aider", "cline"} {
		caps := Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule}}
		_ = ReportUnsupported(caps, bA, OnUnsupportedWarn)
	}
	first := CapabilityWarningsDigest()
	ResetCapabilityWarnings()
	// Reverse the order of adapters; digest must be identical.
	for _, target := range []string{"cline", "aider"} {
		caps := Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule}}
		_ = ReportUnsupported(caps, bA, OnUnsupportedWarn)
	}
	second := CapabilityWarningsDigest()
	if first == "" || first != second {
		t.Errorf("digest must be stable across input order, got %q vs %q", first, second)
	}
}

func TestCapabilityWarningsDigest_EmptyWhenNoPending(t *testing.T) {
	swapWarner(t)
	if got := CapabilityWarningsDigest(); got != "" {
		t.Errorf("expected empty digest with no pending warnings, got %q", got)
	}
}

func TestCapabilityWarningsDigest_ChangesWithCount(t *testing.T) {
	swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	_ = ReportUnsupported(caps, spec.Bundle{Hooks: []spec.Entry{{Name: "h"}}}, OnUnsupportedWarn)
	one := CapabilityWarningsDigest()
	ResetCapabilityWarnings()
	_ = ReportUnsupported(caps, spec.Bundle{Hooks: []spec.Entry{{Name: "h"}, {Name: "h2"}}}, OnUnsupportedWarn)
	two := CapabilityWarningsDigest()
	if one == two {
		t.Errorf("digest must change when count changes, got identical %q", one)
	}
}

func TestReportUnsupported_FlushClearsBuffer(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	first := buf.String()
	FlushCapabilityWarnings()
	if buf.String() != first {
		t.Errorf("second flush should be no-op, got extra output:\n%s", buf.String())
	}
}

func droppedAgentFieldNotes(t *testing.T, c Capabilities, agents ...spec.Entry) string {
	t.Helper()
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	c.Supports = []spec.Kind{spec.KindAgent}
	if err := ReportUnsupported(c, spec.Bundle{Agents: agents}, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	return buf.String()
}

// A target without a per-agent key for effort or mcpServers must say so,
// instead of dropping the field silently on some targets and noting it on
// others (#1072).
func TestReportUnsupported_NotesDroppedAgentFields(t *testing.T) {
	got := droppedAgentFieldNotes(t, Capabilities{Target: "kilo"},
		spec.Entry{Name: "a", Meta: map[string]any{"effort": "high", "mcpServers": []any{"github"}}},
		spec.Entry{Name: "b", Meta: map[string]any{"effort": "low"}},
		spec.Entry{Name: "c", Meta: map[string]any{}},
	)
	for _, want := range []string{
		"`effort` on 2 agents has no effect on kilo (the agent file has no reasoning effort key)",
		"`mcpServers` on 1 agent has no effect on kilo (the agent file has no MCP server list)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestReportUnsupported_AgentFieldsCarriedOrPassedThroughStaySilent(t *testing.T) {
	agents := []spec.Entry{
		{Name: "a", Meta: map[string]any{"effort": map[string]any{"claude": "xhigh"}}},
		{Name: "b", Meta: map[string]any{"mcpServers": []any{"github"}, "x-kiro": map[string]any{"mcpServers": map[string]any{}}}},
	}
	if got := droppedAgentFieldNotes(t, Capabilities{Target: "claude", AgentFields: []string{"effort", "mcpServers"}}, agents...); got != "" {
		t.Errorf("claude carries both fields, got notes:\n%s", got)
	}
	// effort resolves to nothing for kiro, and x-kiro.mcpServers writes the
	// native key, so neither field is dropped.
	if got := droppedAgentFieldNotes(t, Capabilities{Target: "kiro"}, agents...); got != "" {
		t.Errorf("kiro drops nothing here, got notes:\n%s", got)
	}
}

func TestReportUnsupported_AgentFieldReasonOverridesDefault(t *testing.T) {
	got := droppedAgentFieldNotes(t, Capabilities{Target: "kiro", AgentFieldReasons: map[string]string{"mcpServers": "set x-kiro.mcpServers"}},
		spec.Entry{Name: "a", Meta: map[string]any{"mcpServers": []any{"github"}}})
	if !strings.Contains(got, "`mcpServers` on 1 agent has no effect on kiro (set x-kiro.mcpServers)") {
		t.Errorf("expected the target reason, got:\n%s", got)
	}
}

// The hint names the real fix first: a warning about a target nobody uses
// goes away when the target leaves `targets:`. Silencing is the fallback.
func TestFlushCapabilityWarnings_HintSuggestsDroppingUnusedTargets(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	if err := ReportUnsupported(caps, spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	got := buf.String()
	for _, want := range []string{"remove targets you do not use from `targets:`", "`on-unsupported: silent`"} {
		if !strings.Contains(got, want) {
			t.Errorf("hint should mention %q, got:\n%s", want, got)
		}
	}
}
