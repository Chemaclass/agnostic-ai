package emit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func swapWarnerForNotes(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := Warner
	Warner = buf
	t.Cleanup(func() { Warner = prev })
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	return buf
}

func TestNoteCoverageGap_FlushRendersOneLinePerGroup(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	if buf.Len() != 0 {
		t.Fatalf("notes must buffer until flush, got early output: %s", buf)
	}
	FlushCoverageNotes()
	got := buf.String()
	want := "  note: 2 skills reach gemini only via outputs.gemini.emit-skills-as-commands\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestNoteCoverageGap_ZeroCountBuffersNothing(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 0, "outputs.gemini.emit-skills-as-commands")
	if got := PendingCoverageNotesCount(); got != 0 {
		t.Fatalf("zero-count note must not buffer, count=%d", got)
	}
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output for zero-count note, got: %s", buf)
	}
}

func TestNoteCoverageGap_GroupsSharedKindCountViaAcrossTargets(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.x.emit-skills-as-commands")
	NoteCoverageGap("opencode", spec.KindSkill, 2, "outputs.x.emit-skills-as-commands")
	FlushCoverageNotes()
	got := buf.String()
	want := "  note: 2 skills reach gemini, opencode only via outputs.x.emit-skills-as-commands\n"
	if got != want {
		t.Errorf("expected grouped targets on one line, got %q", got)
	}
}

// A free-text reason (no `outputs.` key) renders as a source-dir clause
// instead of "via <reason>".
func TestNoteCoverageGap_FreeTextReasonRendersAsSourceDir(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("warp", spec.KindSkill, 1, "no native skill surface")
	FlushCoverageNotes()
	want := "  note: 1 skill reaches warp only in the source dir (no native skill surface)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected source-dir clause, got %q", got)
	}
}

func TestNoteCoverageGap_PluralizesSingular(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("zed", spec.KindHook, 1, "outputs.zed.tasks-file")
	FlushCoverageNotes()
	if !strings.Contains(buf.String(), "1 hook reaches zed only via outputs.zed.tasks-file") {
		t.Errorf("expected singular kind for n=1, got: %s", buf)
	}
}

func TestNoteCoverageGap_DedupSameTargetKindViaWithinFlush(t *testing.T) {
	buf := swapWarnerForNotes(t)
	for i := 0; i < 3; i++ {
		NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	}
	FlushCoverageNotes()
	if strings.Count(buf.String(), "reach gemini") != 1 {
		t.Errorf("expected a single line for repeat notes, got: %s", buf)
	}
}

func TestCoverageNotesDigest_StableAcrossOrder(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	NoteCoverageGap("opencode", spec.KindSkill, 2, "outputs.opencode.emit-skills-as-commands")
	first := CoverageNotesDigest()
	ResetCoverageNotes()
	NoteCoverageGap("opencode", spec.KindSkill, 2, "outputs.opencode.emit-skills-as-commands")
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	second := CoverageNotesDigest()
	if first == "" || first != second {
		t.Errorf("digest must be stable across input order, got %q vs %q", first, second)
	}
}

func TestCoverageNotesDigest_EmptyWhenNoPending(t *testing.T) {
	swapWarnerForNotes(t)
	if got := CoverageNotesDigest(); got != "" {
		t.Errorf("expected empty digest with no pending notes, got %q", got)
	}
}

func TestCoverageNotesDigest_ChangesWithCount(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")
	one := CoverageNotesDigest()
	ResetCoverageNotes()
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	two := CoverageNotesDigest()
	if one == two {
		t.Errorf("digest must change when count changes, got identical %q", one)
	}
}

func TestFlushCoverageNotes_ClearsBuffer(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 2, "outputs.gemini.emit-skills-as-commands")
	FlushCoverageNotes()
	first := buf.String()
	FlushCoverageNotes()
	if buf.String() != first {
		t.Errorf("second flush should be a no-op, got extra output: %s", buf)
	}
}

// NoteFieldNoOp covers a different failure shape than NoteCoverageGap: the
// entry itself reaches the target in full, only one attribute inside it
// is inert once it lands. The rendered sentence must never borrow
// NoteCoverageGap's "reaches ... only in the source dir" phrasing, which
// would falsely imply the whole entry never reached the target.

func TestNoteFieldNoOp_FlushRendersOneLinePerGroup(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "no file-based way to pre-disable a project-scoped MCP server")
	if buf.Len() != 0 {
		t.Fatalf("field notes must buffer until flush, got early output: %s", buf)
	}
	FlushCoverageNotes()
	got := buf.String()
	want := "  note: `disabled` on 1 mcp has no effect on claude (no file-based way to pre-disable a project-scoped MCP server)\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestNoteFieldNoOp_NeverClaimsEntryMissedTheTarget(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "no file-based way to pre-disable a project-scoped MCP server")
	FlushCoverageNotes()
	got := buf.String()
	for _, forbidden := range []string{"reaches claude only", "in the source dir"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("field no-op must not read as a missing entry, found %q in: %s", forbidden, got)
		}
	}
}

func TestNoteFieldNoOp_ZeroCountBuffersNothing(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 0, "reason")
	if got := PendingCoverageNotesCount(); got != 0 {
		t.Fatalf("zero-count field note must not buffer, count=%d", got)
	}
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output for zero-count field note, got: %s", buf)
	}
}

func TestNoteFieldNoOp_GroupsSameFieldAcrossTargets(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "no file-based way to pre-disable a project-scoped MCP server")
	NoteFieldNoOp("cursor", spec.KindMCP, "disabled", 1, "no file-based way to pre-disable a project-scoped MCP server")
	FlushCoverageNotes()
	want := "  note: `disabled` on 1 mcp has no effect on claude, cursor (no file-based way to pre-disable a project-scoped MCP server)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected grouped targets on one line, got %q", got)
	}
}

// A whole-entry gap and a field no-op for the same target/kind must not
// merge into one line; they describe different failure shapes.
func TestNoteFieldNoOp_DoesNotMergeWithCoverageGap(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteCoverageGap("claude", spec.KindMCP, 1, "no native surface")
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "no file-based way to pre-disable a project-scoped MCP server")
	FlushCoverageNotes()
	got := buf.String()
	if strings.Count(got, "\n") != 2 {
		t.Errorf("expected two distinct note lines, got:\n%s", got)
	}
}

func TestNoteFieldNoOp_ResetClearsBuffer(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "reason")
	ResetCoverageNotes()
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected reset to drop buffered field notes, got: %s", buf)
	}
}

func TestCoverageNotesDigest_ChangesWhenFieldNoOpAdded(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")
	withoutField := CoverageNotesDigest()
	NoteFieldNoOp("claude", spec.KindMCP, "disabled", 1, "reason")
	withField := CoverageNotesDigest()
	if withoutField == withField {
		t.Errorf("digest must change when a field no-op is added, got identical %q", withoutField)
	}
}
