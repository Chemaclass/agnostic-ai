package emit

import (
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestAcceptCoverageNotes_MatchesFieldViaAndSurface(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("codex", spec.KindAgent, "tools", 2, "Codex uses tools as a table")
	NoteFieldNoOp("codex", spec.KindAgent, "effort", 1, "no effort key")
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "bash only")
	NoteProject("a project note")

	accepted, unmatched := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: config.CoverageTargets{"codex"}, Kind: "agents", Field: "tools", Reason: "sandbox_mode limits Codex"},
		{Target: config.CoverageTargets{"gemini"}, Kind: "skills", Via: "outputs.gemini.emit-skills-as-commands", Reason: "skills stay commands-free"},
		{Target: config.CoverageTargets{"copilot"}, Kind: "hooks", Surface: "Copilot cloud agent", Reason: "local hooks only"},
		{Target: config.CoverageTargets{"codex"}, Kind: "agents", Field: "mcpServers", Reason: "stale"},
	})

	if len(accepted) != 3 {
		t.Fatalf("want 3 accepted notes, got %+v", accepted)
	}
	want := AcceptedNote{Text: "`tools` on 2 agents has no effect on codex (Codex uses tools as a table)", Reason: "sandbox_mode limits Codex"}
	if !slices.Contains(accepted, want) {
		t.Errorf("want %+v among %+v", want, accepted)
	}
	if len(unmatched) != 1 || unmatched[0].Field != "mcpServers" {
		t.Errorf("want the mcpServers entry unmatched, got %+v", unmatched)
	}
	if got := PendingTargetCoverageNotesCount(); got != 1 {
		t.Errorf("only the effort note should stay pending, got %d", got)
	}
	FlushCoverageNotes()
	out := buf.String()
	if strings.Contains(out, "`tools`") || strings.Contains(out, "gemini") || strings.Contains(out, "Copilot cloud agent") {
		t.Errorf("accepted notes still flushed:\n%s", out)
	}
	if !strings.Contains(out, "`effort` on 1 agent") || !strings.Contains(out, "a project note") {
		t.Errorf("unaccepted notes should still flush:\n%s", out)
	}
}

// An entry names one note, never every note of the kind: a second gap or
// surface note on the same target and kind stays.
func TestAcceptCoverageNotes_OtherGapAndSurfaceNotesOfTheKindStay(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("continue", spec.KindMCP, 1, "remote servers need a url")
	NoteCoverageGap("continue", spec.KindMCP, 1, "incomplete server entries")
	NoteSurfaceGap("cursor", spec.KindReview, 1, "Bugbot", "too long")
	NoteSurfaceGap("cursor", spec.KindReview, 1, "Cursor review", "over budget")

	accepted, _ := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: config.CoverageTargets{"continue"}, Kind: "mcps", Via: "remote servers need a url", Reason: "r"},
		{Target: config.CoverageTargets{"cursor"}, Kind: "reviews", Surface: "Bugbot", Reason: "r"},
	})

	if len(accepted) != 2 {
		t.Errorf("want 2 accepted notes, got %+v", accepted)
	}
	if got := PendingTargetCoverageNotesCount(); got != 2 {
		t.Errorf("the other gap and surface notes should stay pending, got %d", got)
	}
}

// A field entry covers every reason that field has on the target.
func TestAcceptCoverageNotes_FieldEntryCoversEveryReason(t *testing.T) {
	swapWarnerForNotes(t)
	NoteFieldNoOp("copilot", spec.KindHook, "matcher", 1, "camelCase trap")
	NoteFieldNoOp("copilot", spec.KindHook, "matcher", 2, "not a tool event")

	accepted, unmatched := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: config.CoverageTargets{"copilot"}, Kind: "hooks", Field: "matcher", Reason: "r"},
	})

	if len(accepted) != 2 || len(unmatched) != 0 {
		t.Errorf("want both matcher notes accepted: accepted=%+v unmatched=%+v", accepted, unmatched)
	}
	if got := PendingTargetCoverageNotesCount(); got != 0 {
		t.Errorf("no matcher note should stay pending, got %d", got)
	}
}

// One entry covers a field note across several targets, and reports the
// targets it matched nothing on.
func TestAcceptCoverageNotes_TargetListMatchesEachTarget(t *testing.T) {
	swapWarnerForNotes(t)
	NoteFieldNoOp("codex", spec.KindSkill, "argument-hint", 1, "no hint key")
	NoteFieldNoOp("gemini", spec.KindSkill, "argument-hint", 1, "no hint key")

	accepted, unmatched := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: config.CoverageTargets{"codex", "gemini", "cursor"}, Kind: "skills", Field: "argument-hint", Reason: "r"},
	})

	if len(accepted) != 2 {
		t.Errorf("want one accepted note per target, got %+v", accepted)
	}
	if len(unmatched) != 1 || !slices.Equal(unmatched[0].Target, config.CoverageTargets{"cursor"}) {
		t.Errorf("want the entry unmatched on cursor only, got %+v", unmatched)
	}
}

func TestPrintAcceptedNotes_NamesTheReason(t *testing.T) {
	buf := swapWarnerForNotes(t)
	PrintAcceptedNotes([]AcceptedNote{{Text: "`tools` on 1 agent has no effect on codex (no allowlist)", Reason: "known"}})
	if got, want := buf.String(), "  accepted: `tools` on 1 agent has no effect on codex (no allowlist)\n    reason: known\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Two targets that both ignore an environment field each get a note.
// Accepting one must leave the other in place.
func TestAcceptCoverageNotes_KeepsOtherTargetsEnvironmentNote(t *testing.T) {
	buf := swapWarnerForNotes(t)
	env := []spec.Entry{{Kind: spec.KindEnvironment, Name: "dev", Meta: map[string]any{"tasks": "x"}}}
	RecordEnvironmentFields("codex", env)
	RecordEnvironmentFields("cursor", env)
	NoteFieldNoOp("codex", spec.KindEnvironment, "tasks", 1, "no file for it")
	NoteFieldNoOp("cursor", spec.KindEnvironment, "tasks", 1, "no file for it")

	AcceptCoverageNotes([]config.CoverageAccept{
		{Target: config.CoverageTargets{"codex"}, Kind: "environments", Field: "tasks", Reason: "known"},
	})

	if got := PendingTargetCoverageNotesCount(); got != 1 {
		t.Errorf("the cursor note should stay pending, got %d", got)
	}
	FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`tasks` on 1 environment has no effect on cursor") {
		t.Errorf("the unaccepted cursor note should flush:\n%s", buf)
	}
}
