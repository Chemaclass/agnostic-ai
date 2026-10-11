package opencode

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
	for _, key := range []string{"license", "disable-model-invocation", "metadata"} {
		if value, ok := resolved[key]; ok {
			meta[key] = value
			keys = append(keys, key)
		}
	}
	exclude := append(append([]string(nil), keys...), "model", "effort")
	emit.MergeCustomTargetMeta(meta, &keys, skill.Meta, target, exclude...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(skill.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
