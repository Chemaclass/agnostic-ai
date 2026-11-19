package emit

import (
	"fmt"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// BenchmarkNoteDroppedSkillFields measures the dropped-field check over
// 100 skills, which renders each skill and parses the result back.
//
//	status-quo: NoteDroppedSkillFields
//	baseline:   the render alone, which the write path repeats anyway
func BenchmarkNoteDroppedSkillFields(b *testing.B) {
	skills := make([]spec.Entry, 100)
	for i := range skills {
		name := fmt.Sprintf("skill-%04d", i)
		skills[i] = spec.Entry{
			Kind: spec.KindSkill,
			Name: name,
			Meta: map[string]any{
				"name":          name,
				"description":   fmt.Sprintf("Bench skill %d for deterministic fixtures.", i),
				"allowed-tools": []any{"Read", "Grep"},
			},
			MetaKeys: []string{"name", "description", "allowed-tools"},
			Body:     "Steps: read the target, parse it, report violations.\n",
		}
	}
	b.Run("status-quo", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			NoteDroppedSkillFields("claude", skills)
		}
	})
	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, skill := range skills {
				SkillMarkdown(skill, "claude")
			}
		}
	})
}
