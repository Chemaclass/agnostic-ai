package claude

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) SkillMarkdown(skill spec.Entry) string {
	return emit.DocumentStyled(skill.Meta, skill.MetaKeys, skill.MetaStyles, skill.Body, target)
}
