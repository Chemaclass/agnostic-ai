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

// sharedSkillFields is what Qwen writes into a skills folder another
// target writes too, where every writer must agree on the bytes.
var sharedSkillFields = []string{"license"}

func (Adapter) SkillMarkdown(skill spec.Entry) string {
	return skillMarkdown(skill, skillFields)
}

func sharedSkillMarkdown(skill spec.Entry) string {
	return skillMarkdown(skill, sharedSkillFields)
}

func skillMarkdown(skill spec.Entry, fields []string) string {
	resolved := emit.ResolveMeta(skill.Meta, target)
	description, _ := resolved["description"].(string)
	if description == "" {
		description = skill.Name
	}
	meta := map[string]any{"name": skill.Name, "description": description}
	keys := []string{"name", "description"}
	for _, key := range fields {
		value, ok := resolved[key]
		if !ok {
			continue
		}
		// Qwen rejects a skill whose paths is not a list, so the
		// comma-separated string form becomes one.
		if key == "paths" {
			globs := spec.GlobList(value)
			if len(globs) == 0 {
				continue
			}
			value = globs
		}
		meta[key] = value
		keys = append(keys, key)
	}
	emit.MergeCustomTargetMeta(meta, &keys, skill.Meta, target, append([]string{"name", "description", "effort"}, fields...)...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(skill.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
