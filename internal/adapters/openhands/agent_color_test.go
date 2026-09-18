package openhands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// OpenHands documents `color` on a file-based agent ("Rich color name
// ... used by visualizers to style this agent's output in terminal
// panels", docs.openhands.dev/sdk/guides/agent-file-based). The shared
// `.agents/agents` renderer has no key for it, because Goose reads the
// same tree and its own frontmatter is `name`, `description`, `model`
// only, so writing `color` there would leave a dead key in every Goose
// agent file. The floor is a coverage note naming the field, not
// silence.
func TestEmit_AgentColorSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body",
			Meta: map[string]any{"color": "blue"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`color`", "1 agent", "openhands", "x-openhands"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}

	got := readFile(t, filepath.Join(dir, ".agents/agents/alpha.md"))
	if strings.Contains(got, "color:") {
		t.Errorf("color must not reach the shared agents tree, got: %s", got)
	}
}

// An explicit x-openhands.color is the author choosing target-specific
// behavior, so it reaches the file and raises no note.
func TestEmit_AgentColorUnderXOpenhandsEmitsAndStaysQuiet(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "alpha body",
			Meta: map[string]any{"color": "blue", "x-openhands": map[string]any{"color": "green"}},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); strings.Contains(note, "`color`") {
		t.Errorf("explicit x-openhands.color should raise no note, got: %s", note)
	}
	got := readFile(t, filepath.Join(dir, ".agents/agents/alpha.md"))
	if !strings.Contains(got, "color: green") {
		t.Errorf("expected x-openhands.color in the file, got: %s", got)
	}
}
