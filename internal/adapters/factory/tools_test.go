package factory

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Droid CLI's tool IDs are its own vocabulary: "Arrays must use valid
// IDs from this table or exact registered MCP tool IDs. Unknown IDs
// cause a validation error" (docs.factory.ai/harness/subagents). Seven
// Claude-style names are already valid IDs and carry over; Bash, Write,
// and WebFetch are not, and translate onto Execute, Create, and FetchUrl.
func TestEmit_Agent_ToolsTranslateToFactoryIDs(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "reviewer",
			Meta: map[string]any{
				"description": "Reviews diffs.",
				"tools":       []any{"Read", "LS", "Grep", "Glob", "Write", "Edit", "ApplyPatch", "Bash", "WebSearch", "WebFetch"},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/reviewer.md"))
	for _, want := range []string{
		"- Read", "- LS", "- Grep", "- Glob", "- Create", "- Edit",
		"- ApplyPatch", "- Execute", "- WebSearch", "- FetchUrl",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Bash", "Write", "WebFetch"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q is not a Factory tool ID and must never be written verbatim, got:\n%s", unwanted, got)
		}
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when every name reaches a Factory ID, got %d", n)
	}
}

// Two Claude-style names can land on the same Factory ID (Bash and a
// literal Execute both mean Execute). DroidValidator warns on duplicate
// tools, so the translated list dedupes in first-seen order.
func TestEmit_Agent_ToolsDedupeAfterTranslation(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Bash", "Execute"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if n := strings.Count(got, "- Execute"); n != 1 {
		t.Errorf("expected Execute listed once, got %d in:\n%s", n, got)
	}
}

// "TodoWrite and Skill are always included for every droid ... You do
// not list them, and they do not appear in the tool count." Dropping
// them costs the droid nothing, so no note fires as long as some other
// name survives.
func TestEmit_Agent_AlwaysOnToolsDropWithoutNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read", "TodoWrite", "Skill"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if !strings.Contains(got, "- Read") {
		t.Errorf("expected Read to survive, got:\n%s", got)
	}
	for _, unwanted := range []string{"TodoWrite", "Skill"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q is always on and must not be listed, got:\n%s", unwanted, got)
		}
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note when the only dropped names are always-on ones, got %d", n)
	}
}

// "ExitSpecMode and GenerateDroid cannot be enabled by a custom droid;
// listing either one is a validation error." Neither has a table entry,
// so both drop and the drop surfaces a note.
func TestEmit_Agent_ForbiddenToolsDropAndNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read", "ExitSpecMode", "GenerateDroid"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if !strings.Contains(got, "- Read") {
		t.Errorf("expected Read to still emit alongside the forbidden names, got:\n%s", got)
	}
	for _, unwanted := range []string{"ExitSpecMode", "GenerateDroid"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q is forbidden for a custom droid and must never be written, got:\n%s", unwanted, got)
		}
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note for the forbidden names, got %d", n)
	}
}

// A name with no Factory ID is dropped rather than written unconfirmed,
// and the drop surfaces as one note per sync naming the vendor table.
func TestEmit_Agent_UnknownToolDropsAndNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prevWarner := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prevWarner })

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"Read", "NotebookEdit"}}, Body: "body"},
		{Kind: spec.KindAgent, Name: "a2", Meta: map[string]any{"tools": []any{"Read"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if strings.Contains(got, "NotebookEdit") {
		t.Errorf("expected the unknown name to never be written verbatim, got:\n%s", got)
	}
	if !strings.Contains(got, "- Read") {
		t.Errorf("expected the valid name to still emit, got:\n%s", got)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "`tools` on 1 agent has no effect on factory") {
		t.Errorf("expected a note naming the one agent with an unknown tool, got: %s", out)
	}
	if !strings.Contains(out, "Factory's tool-ID table") {
		t.Errorf("expected the note to name the vendor tool-ID table, got: %s", out)
	}
}

// A list of nothing but always-on names leaves no ID to emit. Omitting
// `tools` means "allow every tool" on Factory, so the author's
// restriction is gone: that must not pass silently.
func TestEmit_Agent_AllNamesDroppedOmitsKeyAndNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": []any{"TodoWrite"}}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if strings.Contains(got, "tools:") {
		t.Errorf("expected no tools key when no name survives translation, got:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 1 {
		t.Errorf("expected one coverage note when the whole restriction is lost, got %d", n)
	}
}

// "The literal value `tools: all` is rejected. Omit the `tools` field
// entirely to allow every tool." A scalar `all` is not a list, so it
// never reaches the frontmatter, and omission is exactly what the
// author asked for.
func TestEmit_Agent_ToolsAllScalarNeverEmitted(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "a1", Meta: map[string]any{"tools": "all"}, Body: "body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/a1.md"))
	if strings.Contains(got, "tools:") {
		t.Errorf("Droid CLI rejects the literal `tools: all`; expected the key omitted, got:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note: omitting the key is what `all` means, got %d", n)
	}
}

// x-factory.tools is trusted to already be Factory's own vocabulary, so
// it wins outright over the generic list instead of merging with a
// translated copy, and it costs no coverage note.
func TestEmit_Agent_XFactoryToolsOverrideWinsOutright(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{
				"description": "d",
				"tools":       []any{"Bash"},
				"x-factory":   map[string]any{"tools": []any{"LS", "playwright__browser_click"}},
			},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/alpha.md"))
	for _, want := range []string{"- LS", "- playwright__browser_click"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected x-factory.tools to pass through, missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Execute") {
		t.Errorf("expected x-factory.tools to win outright, not merge with the translated generic value, got:\n%s", got)
	}
	if strings.Count(got, "tools:") != 1 {
		t.Errorf("expected exactly one tools key, got:\n%s", got)
	}
	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("expected no coverage note once x-factory.tools rescues the field, got %d", n)
	}
}

// Factory also accepts a category name in place of an array (for
// example `tools: read-only`), a shape the cross-tool list cannot
// express. x-factory.tools carries it through untouched.
func TestEmit_Agent_XFactoryToolsCategoryStringPassesThrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{"description": "d", "x-factory": map[string]any{"tools": "read-only"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/alpha.md"))
	if !strings.Contains(got, "tools: read-only") {
		t.Errorf("expected the category name to pass through, got:\n%s", got)
	}
}

// The translation table is the single place Factory's vocabulary lives;
// a unit-level check pins the three renames and both drop rules without
// going through a file write.
func TestTranslateTools(t *testing.T) {
	cases := []struct {
		name        string
		in          []string
		want        []string
		wantDropped bool
	}{
		{"renames", []string{"Bash", "Write", "WebFetch"}, []string{"Execute", "Create", "FetchUrl"}, false},
		{"vendor IDs pass through", []string{"LS", "ApplyPatch", "FetchUrl"}, []string{"LS", "ApplyPatch", "FetchUrl"}, false},
		{"dedupes", []string{"Write", "Create"}, []string{"Create"}, false},
		{"case sensitive", []string{"bash"}, nil, true},
		{"always-on names drop quietly", []string{"Read", "TodoWrite", "Skill"}, []string{"Read"}, false},
		{"forbidden names drop loudly", []string{"ExitSpecMode", "GenerateDroid"}, nil, true},
		{"unknown name drops loudly", []string{"NotebookEdit"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, dropped := translateTools(c.in)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("translateTools(%v) = %v, want %v", c.in, got, c.want)
			}
			if dropped != c.wantDropped {
				t.Errorf("translateTools(%v) dropped = %v, want %v", c.in, dropped, c.wantDropped)
			}
		})
	}
}
