package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestDroppedByTarget_GroupsAndDoesNotClear(t *testing.T) {
	ResetCapabilityWarnings()
	ResetCoverageNotes()
	t.Cleanup(ResetCapabilityWarnings)
	t.Cleanup(ResetCoverageNotes)

	// cursor: hooks unsupported (dropped). gemini: skills downgraded.
	caps := Capabilities{Target: "cursor", Supports: []spec.Kind{spec.KindRule}}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindHook, Name: "h", Meta: map[string]any{"event": "PostToolUse"}},
	})
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	NoteCoverageGap("gemini", spec.KindSkill, 3, "outputs.gemini.emit-skills-as-commands")

	drops := DroppedByTarget()
	if len(drops) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(drops), drops)
	}
	// sorted by target name: cursor, gemini
	if drops[0].Target != "cursor" || len(drops[0].Unsupported) != 1 || drops[0].Unsupported[0].Kind != spec.KindHook {
		t.Errorf("cursor unsupported wrong: %+v", drops[0])
	}
	if drops[1].Target != "gemini" || len(drops[1].Downgraded) != 1 || drops[1].Downgraded[0].Count != 3 {
		t.Errorf("gemini downgraded wrong: %+v", drops[1])
	}

	// Read-only: buffers still populated for the regular flushes.
	if PendingCapabilityWarningsCount() == 0 {
		t.Errorf("DroppedByTarget must not clear the capability buffer")
	}
	if PendingCoverageNotesCount() == 0 {
		t.Errorf("DroppedByTarget must not clear the coverage buffer")
	}
}

func TestRenderDroppedSummary_Output(t *testing.T) {
	ResetCapabilityWarnings()
	ResetCoverageNotes()
	t.Cleanup(ResetCapabilityWarnings)
	t.Cleanup(ResetCoverageNotes)

	caps := Capabilities{Target: "cursor", Supports: []spec.Kind{spec.KindRule}}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"model": "x"}},
	})
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	NoteCoverageGap("aider", spec.KindAgent, 2, "outputs.aider.rules-file")
	NoteCoverageGap("aider", spec.KindSkill, 1, "source-dir only") // free-text via

	var sb strings.Builder
	RenderDroppedSummary(&sb)
	out := sb.String()
	if !strings.Contains(out, "cursor: 1 settings dropped (unsupported)") {
		t.Errorf("missing cursor unsupported line: %q", out)
	}
	// Config-key via renders "via …"; free-text via renders "in the source dir (…)".
	if !strings.Contains(out, "2 agents via outputs.aider.rules-file") {
		t.Errorf("missing config-key downgrade: %q", out)
	}
	if !strings.Contains(out, "1 skill in the source dir (source-dir only)") {
		t.Errorf("missing free-text downgrade wording: %q", out)
	}
	// Both aider downgrades collapse onto one line joined by "; ".
	if !strings.Contains(out, "aider: ") || !strings.Contains(out, "; ") {
		t.Errorf("expected single joined aider line: %q", out)
	}
}

// DroppedByTarget collapses duplicate (target,kind) warnings and
// (target,kind,via) notes the same way the flushes do, and sorts kinds
// within a target.
func TestDroppedByTarget_DedupAndIntraTargetSort(t *testing.T) {
	ResetCapabilityWarnings()
	ResetCoverageNotes()
	t.Cleanup(ResetCapabilityWarnings)
	t.Cleanup(ResetCoverageNotes)

	caps := Capabilities{Target: "cursor", Supports: []spec.Kind{spec.KindRule}}
	// Two unsupported kinds out of alphabetical order (settings, hook), each
	// reported twice to exercise the dedup.
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSettings, Name: "s", Meta: map[string]any{"model": "x"}},
		{Kind: spec.KindHook, Name: "h", Meta: map[string]any{"event": "PostToolUse"}},
	})
	for i := 0; i < 2; i++ {
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	drops := DroppedByTarget()
	if len(drops) != 1 {
		t.Fatalf("expected 1 target, got %d", len(drops))
	}
	got := drops[0].Unsupported
	if len(got) != 2 {
		t.Fatalf("dedup failed: expected 2 distinct kinds, got %d: %+v", len(got), got)
	}
	if got[0].Kind != spec.KindHook || got[1].Kind != spec.KindSettings {
		t.Errorf("intra-target sort wrong: %+v", got)
	}
}

func TestRenderDroppedSummary_EmptyWhenNothingBuffered(t *testing.T) {
	ResetCapabilityWarnings()
	ResetCoverageNotes()
	var sb strings.Builder
	RenderDroppedSummary(&sb)
	if sb.Len() != 0 {
		t.Errorf("expected no output, got %q", sb.String())
	}
}
