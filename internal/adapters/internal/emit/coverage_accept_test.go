package emit

import (
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestAcceptCoverageNotes_MatchesFieldGapAndSurfaceNotes(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("codex", spec.KindAgent, "tools", 2, "Codex uses tools as a table")
	NoteFieldNoOp("codex", spec.KindAgent, "effort", 1, "no effort key")
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "bash only")
	NoteProject("a project note")

	accepted, unmatched := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: "codex", Kind: "agents", Field: "tools", Reason: "sandbox_mode limits Codex"},
		{Target: "gemini", Kind: "skills", Reason: "skills stay commands-free"},
		{Target: "copilot", Kind: "hooks", Reason: "local hooks only"},
		{Target: "codex", Kind: "agents", Field: "mcpServers", Reason: "stale"},
	})

	if len(accepted) != 3 {
		t.Fatalf("want 3 accepted notes, got %+v", accepted)
	}
	want := AcceptedNote{Text: "`tools` on 2 agents has no effect on codex", Reason: "sandbox_mode limits Codex"}
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

func TestAcceptCoverageNotes_FieldEntryDoesNotMatchWholeKindNote(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")

	accepted, unmatched := AcceptCoverageNotes([]config.CoverageAccept{
		{Target: "gemini", Kind: "skills", Field: "name", Reason: "r"},
	})

	if len(accepted) != 0 || len(unmatched) != 1 {
		t.Errorf("a field entry must not accept a whole-kind note: accepted=%+v unmatched=%+v", accepted, unmatched)
	}
}

func TestPrintAcceptedNotes_NamesTheReason(t *testing.T) {
	buf := swapWarnerForNotes(t)
	PrintAcceptedNotes([]AcceptedNote{{Text: "`tools` on 1 agent has no effect on codex", Reason: "known"}})
	if got, want := buf.String(), "  accepted: `tools` on 1 agent has no effect on codex (reason: known)\n"; got != want {
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
		{Target: "codex", Kind: "environments", Field: "tasks", Reason: "known"},
	})

	if got := PendingTargetCoverageNotesCount(); got != 1 {
		t.Errorf("the cursor note should stay pending, got %d", got)
	}
	FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`tasks` on 1 environment has no effect on cursor") {
		t.Errorf("the unaccepted cursor note should flush:\n%s", buf)
	}
}
