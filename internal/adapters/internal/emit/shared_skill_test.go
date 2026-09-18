package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// A portable frontmatter field set on a skill is dropped by
// SkillMarkdown, which writes only name and description. The predicate
// behind the coverage note has to say so, and has to stay quiet once
// the author has chosen the target's behavior explicitly.
func TestSharedSkillFieldDropped(t *testing.T) {
	const field = "disable-model-invocation"
	cases := []struct {
		name string
		meta map[string]any
		want bool
	}{
		{"field set, no override", map[string]any{field: true}, true},
		{"field set false is still set", map[string]any{field: false}, true},
		{"field unset", map[string]any{"description": "d"}, false},
		{"explicit target override", map[string]any{
			field: true, XPrefix + "crush": map[string]any{field: true},
		}, false},
		{"explicit nil delete marker", map[string]any{
			field: true, XPrefix + "crush": map[string]any{field: nil},
		}, false},
		{"another target's override does not count", map[string]any{
			field: true, XPrefix + "factory": map[string]any{field: true},
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			skill := spec.Entry{Kind: spec.KindSkill, Name: "deploy", Meta: c.meta}
			if got := SharedSkillFieldDropped(skill, "crush", field); got != c.want {
				t.Errorf("SharedSkillFieldDropped = %v, want %v", got, c.want)
			}
		})
	}
}

// SkillMarkdown is what makes the field disappear; pin that, so the
// note's premise cannot rot without a test noticing.
func TestSkillMarkdown_DropsPortableFieldsBeyondNameAndDescription(t *testing.T) {
	skill := spec.Entry{Kind: spec.KindSkill, Name: "deploy", Meta: map[string]any{
		"description":              "Ship it",
		"disable-model-invocation": true,
	}, Body: "run it"}
	got := SkillMarkdown(skill, "crush")
	if strings.Contains(got, "disable-model-invocation") {
		t.Errorf("shared skill renderer unexpectedly emits the field:\n%s", got)
	}
}
