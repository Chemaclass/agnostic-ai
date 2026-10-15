package crush

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The x-crush key alone collides with every other target that writes
// .agents/skills, so the note must name the skills-dir override too.
func TestEmit_ManualOnlyNoteNamesSkillsDirOverride(t *testing.T) {
	testutil.TempCwd(t)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{{
		Kind: spec.KindSkill, Name: "release", Path: "skills/release/SKILL.md", Body: "body",
		Meta: map[string]any{"description": "d", "disable-model-invocation": true},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	for _, want := range []string{"x-crush.disable-model-invocation", "outputs.crush.skills-dir"} {
		if !strings.Contains(out, want) {
			t.Errorf("note missing %q:\n%s", want, out)
		}
	}
}
