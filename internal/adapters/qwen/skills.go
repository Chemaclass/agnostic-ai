package qwen

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) SkillMarkdown(skill spec.Entry) string {
	resolved := emit.ResolveMeta(skill.Meta, target)
	description, _ := resolved["description"].(string)
	if description == "" {
		description = skill.Name
	}
	meta := map[string]any{"name": skill.Name, "description": description}
	keys := []string{"name", "description"}
	if license, ok := resolved["license"]; ok {
		meta["license"] = license
		keys = append(keys, "license")
	}
	emit.MergeCustomTargetMeta(meta, &keys, skill.Meta, target, "name", "description", "license", "effort")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(skill.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
