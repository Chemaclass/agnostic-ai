package qwen

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// skillFields are the portable SKILL.md keys Qwen Code reads as written
// (qwenlm.github.io/qwen-code-docs/en/users/features/skills/). It reads
// `allowedTools` as a permission grant and ignores `allowed-tools`, so
// that one stays out.
var skillFields = []string{"license", "paths", "disable-model-invocation", "user-invocable"}

func (Adapter) SkillMarkdown(skill spec.Entry) string {
	resolved := emit.ResolveMeta(skill.Meta, target)
	description, _ := resolved["description"].(string)
	if description == "" {
		description = skill.Name
	}
	meta := map[string]any{"name": skill.Name, "description": description}
	keys := []string{"name", "description"}
	for _, key := range skillFields {
		if value, ok := resolved[key]; ok {
			meta[key] = value
			keys = append(keys, key)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, skill.Meta, target, append([]string{"name", "description", "effort"}, skillFields...)...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(skill.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
