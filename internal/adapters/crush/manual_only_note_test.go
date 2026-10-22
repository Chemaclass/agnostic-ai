package crush

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitManualOnlySkill(t *testing.T, sess *emit.Session, extra map[string]any) (skill, notes string) {
	t.Helper()
	dir := testutil.TempCwd(t)
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
	for k, v := range extra {
		entries[0].Meta[k] = v
	}
	if err := New().Emit(sess, spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	return readFile(t, filepath.Join(dir, ".agents/skills/release/SKILL.md")), buf.String()
}

// Crush lists a skill for the user only with both fields set.
func TestEmit_ManualOnlySkillGetsBothCrushFields(t *testing.T) {
	skill, notes := emitManualOnlySkill(t, emit.NewSession(), nil)

	for _, want := range []string{"disable-model-invocation: true", "user-invocable: true"} {
		if !strings.Contains(skill, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, skill)
		}
	}
	if strings.Contains(notes, "disable-model-invocation") {
		t.Errorf("no note when the field is written:\n%s", notes)
	}
}

// Another target writing .agents/skills needs the same bytes, so the
// fields drop with a note.
func TestEmit_ManualOnlyDropsInASharedSkillsFolder(t *testing.T) {
	sess := emit.NewSession()
	sess.SetSkillsDirWriters(map[string][]string{".agents/skills": {"codex", target}})
	skill, notes := emitManualOnlySkill(t, sess, nil)

	if strings.Contains(skill, "disable-model-invocation") || strings.Contains(skill, "user-invocable") {
		t.Errorf("shared SKILL.md must match the other writers:\n%s", skill)
	}
	if !strings.Contains(notes, "`disable-model-invocation` on 1 skill has no effect on crush") {
		t.Errorf("want the dropped field noted:\n%s", notes)
	}
}

// A spec's own user-invocable value reaches the file, so the author's
// choice is kept instead of dropped.
func TestEmit_ManualOnlySkillKeepsTheSpecsUserInvocable(t *testing.T) {
	for _, value := range []bool{true, false} {
		skill, _ := emitManualOnlySkill(t, emit.NewSession(), map[string]any{"user-invocable": value})
		for _, want := range []string{"disable-model-invocation: true", fmt.Sprintf("user-invocable: %t", value)} {
			if !strings.Contains(skill, want) {
				t.Errorf("user-invocable: %t: SKILL.md missing %q:\n%s", value, want, skill)
			}
		}
	}
}
