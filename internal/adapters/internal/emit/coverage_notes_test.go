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
	NoteCoverageGap("kiro", spec.KindSkill, 1, "bundled assets stay in the source dir")
	FlushCoverageNotes()
	want := "  note: 1 skill reaches kiro only in the source dir (bundled assets stay in the source dir)\n"
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

// A surface gap is the third sentence shape: the entry reached the
// target and every field on it is live, so one of the target's own
// surfaces, not the target, is what drops it.

func TestNoteSurfaceGap_FlushRendersOneLinePerGroup(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "exec entries are CLI only")
	if buf.Len() != 0 {
		t.Fatalf("surface notes must buffer until flush, got early output: %s", buf)
	}
	FlushCoverageNotes()
	want := "  note: 1 hook reaches copilot but not Copilot cloud agent (exec entries are CLI only)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestNoteSurfaceGap_PluralizesSubjectAndVerb(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 3, "Copilot cloud agent", "exec entries are CLI only")
	FlushCoverageNotes()
	want := "  note: 3 hooks reach copilot but not Copilot cloud agent (exec entries are CLI only)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// The point of the third shape: it must not borrow either of the other
// two sentences, which would claim the entry never landed or that a
// field is inert everywhere on the target.
func TestNoteSurfaceGap_NeverClaimsTheWholeTargetIgnoresIt(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "exec entries are CLI only")
	FlushCoverageNotes()
	got := buf.String()
	for _, forbidden := range []string{"has no effect on copilot", "in the source dir", "only via"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("surface gap must not read as a target-wide drop, found %q in: %s", forbidden, got)
		}
	}
}

func TestNoteSurfaceGap_ZeroCountBuffersNothing(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 0, "Copilot cloud agent", "reason")
	if got := PendingCoverageNotesCount(); got != 0 {
		t.Fatalf("zero-count surface note must not buffer, count=%d", got)
	}
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output for zero-count surface note, got: %s", buf)
	}
}

func TestNoteSurfaceGap_GroupsSameSurfaceAcrossTargets(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 1, "a cloud sandbox", "exec entries are CLI only")
	NoteSurfaceGap("claude", spec.KindHook, 1, "a cloud sandbox", "exec entries are CLI only")
	FlushCoverageNotes()
	want := "  note: 1 hook reaches copilot, claude but not a cloud sandbox (exec entries are CLI only)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected grouped targets on one line, got %q", got)
	}
}

func TestNoteSurfaceGap_ResetClearsBuffer(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "reason")
	ResetCoverageNotes()
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected reset to drop buffered surface notes, got: %s", buf)
	}
}

func TestCoverageNotesDigest_ChangesWhenSurfaceGapAdded(t *testing.T) {
	swapWarnerForNotes(t)
	NoteCoverageGap("gemini", spec.KindSkill, 1, "outputs.gemini.emit-skills-as-commands")
	withoutSurface := CoverageNotesDigest()
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "reason")
	withSurface := CoverageNotesDigest()
	if withoutSurface == withSurface {
		t.Errorf("digest must change when a surface gap is added, got identical %q", withoutSurface)
	}
}

func TestNoteProject_BuffersDedupesAndDigests(t *testing.T) {
	buf := swapWarnerForNotes(t)
	NoteProject("")
	if got := CoverageNotesDigest(); got != "" {
		t.Fatalf("empty project note must not buffer, digest=%q", got)
	}
	NoteProject("two trees overlap")
	NoteProject("two trees overlap")
	if buf.Len() != 0 {
		t.Fatalf("notes must buffer until flush, got early output: %s", buf)
	}
	if got := PendingCoverageNotesCount(); got != 1 {
		t.Errorf("duplicate project notes must count once, count=%d", got)
	}
	first := CoverageNotesDigest()
	if first == "" {
		t.Fatal("project note must feed the suppression digest")
	}
	FlushCoverageNotes()
	if got, want := buf.String(), "  note: two trees overlap\n"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	NoteProject("three trees overlap")
	if CoverageNotesDigest() == first {
		t.Error("a changed project note must change the digest")
	}
}

// One environment spec feeds each tool its own part, so a field one tool
// ignores and another reads prints no note; a field no tool reads does.
func TestFieldNoOp_EnvironmentFieldReadElsewhereIsDropped(t *testing.T) {
	buf := swapWarnerForNotes(t)
	env := []spec.Entry{{Kind: spec.KindEnvironment, Name: "dev", Meta: map[string]any{
		"cleanup": "make stop", "terminals": []any{"x"}, "tasks": "x",
		"dev-commands": []any{map[string]any{"name": "d", "command": "run", "port": 3000}},
	}}}
	RecordEnvironmentFields("codex", env)
	RecordEnvironmentFields("claude", env)
	RecordEnvironmentFields("cursor", env)
	NoteFieldNoOp("cursor", spec.KindEnvironment, "cleanup", 1, "no cleanup step")
	NoteFieldNoOp("codex", spec.KindEnvironment, "dev-commands.port", 1, "no port field")
	NoteFieldNoOp("codex", spec.KindEnvironment, "terminals", 1, "no terminal list")
	NoteFieldNoOp("claude", spec.KindEnvironment, "terminals", 1, "no terminal list")
	for _, target := range []string{"codex", "claude", "cursor"} {
		NoteFieldNoOp(target, spec.KindEnvironment, "tasks", 1, "no file for it")
	}
	NoteFieldNoOp("cursor", spec.KindAgent, "cleanup", 1, "other kind")

	if got := PendingCoverageNotesCount(); got != 4 {
		t.Errorf("pending notes = %d, want 4: tasks on each target, which none reads, and the agent note", got)
	}
	FlushCoverageNotes()
	got := buf.String()
	for _, gone := range []string{"`cleanup` on 1 environment", "dev-commands.port", "terminals"} {
		if strings.Contains(got, gone) {
			t.Errorf("a field another target reads was noted: %q in\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "`tasks` on 1 environment has no effect on codex, claude, cursor") {
		t.Errorf("a field no target reads must stay noted:\n%s", got)
	}
	if !strings.Contains(got, "`cleanup` on 1 agent has no effect on cursor") {
		t.Errorf("notes on other kinds must stay:\n%s", got)
	}
}

// A target that notes dev-commands as a whole reads none of its fields,
// and a false flag is not a set field.
func TestFieldNoOp_ParentNoteAndFalseFlagDoNotHideANote(t *testing.T) {
	buf := swapWarnerForNotes(t)
	env := []spec.Entry{{Kind: spec.KindEnvironment, Name: "dev", Meta: map[string]any{
		"dev-commands": []any{map[string]any{"name": "d", "command": "run", "port": 3000, "auto-port": false}},
	}}}
	RecordEnvironmentFields("codex", env)
	RecordEnvironmentFields("amp", env)
	NoteFieldNoOp("amp", spec.KindEnvironment, "dev-commands", 1, "no dev server list")
	NoteFieldNoOp("codex", spec.KindEnvironment, "dev-commands.port", 1, "no port field")
	FlushCoverageNotes()
	if !strings.Contains(buf.String(), "dev-commands.port") {
		t.Errorf("no target reads port, so its note must stay:\n%s", buf)
	}
	RecordEnvironmentFields("claude", env)
	if coverageNoteState.environmentFields["claude"]["dev-commands.auto-port"] {
		t.Error("auto-port: false recorded as a set field")
	}
}

// An omitted entry outlives a flush or a discard, so the list of what a
// tool reads after the notes print still skips it (#1666).
func TestNoteEntryOmitted_LastsUntilReset(t *testing.T) {
	swapWarnerForNotes(t)
	NoteEntryOmitted("zed", spec.KindMCP, "gh")

	FlushCoverageNotes()
	DiscardCoverageNotes()
	restore := SetAsideNotes()
	if OmittedEntry("zed", spec.KindMCP, "gh") {
		t.Error("a capture sees the omission it set aside")
	}
	restore()
	if !OmittedEntry("zed", spec.KindMCP, "gh") {
		t.Error("the omission did not outlive the flush, discard, and capture")
	}
	if OmittedEntry("claude", spec.KindMCP, "gh") || OmittedEntry("zed", spec.KindSkill, "gh") {
		t.Error("an omission leaked to another target or kind")
	}
	ResetCoverageNotes()
	if OmittedEntry("zed", spec.KindMCP, "gh") {
		t.Error("reset kept the omission")
	}
}
