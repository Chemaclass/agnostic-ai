package adapters

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type SkillRenderer interface {
	SkillMarkdown(skill spec.Entry) string
}

// RenderSkillMarkdown keeps shared user directories free of target-specific metadata.
func RenderSkillMarkdown(target string, skill spec.Entry, shared bool) (string, error) {
	content := ""
	if shared {
		content = emit.SkillMarkdown(skill, "")
	} else {
		adapter, err := Resolve(target)
		if err != nil {
			return "", err
		}
		skill.Body = skill.BodyFor(target)
		if renderer, ok := adapter.(SkillRenderer); ok {
			content = renderer.SkillMarkdown(skill)
		} else {
			content = emit.SkillMarkdown(skill, target)
		}
	}
	return strings.TrimRight(emit.WithHeader(content, emit.FormatMarkdown), "\n") + "\n", nil
}

func NoteDroppedSkillFields(target string, skills []spec.Entry) {
	emit.NoteDroppedSkillFields(target, (spec.Bundle{Skills: skills}).For(target).Skills)
}
