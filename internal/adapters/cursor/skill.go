package cursor

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSkill writes the native Cursor skill layout:
//
//	<skillsDir>/<name>/
//	  SKILL.md         (required, frontmatter `name` + `description` + body)
//	  <attached files> (any sibling files / subdirs from the source skill)
//
// Cursor discovers skills from `.cursor/skills/<name>/SKILL.md` (one
// folder per skill, the Agent Skills layout it shares with Claude and
// Codex) and supports bundled asset files, so the source skill folder
// propagates byte-for-byte. Beyond the two required fields, the
// documented optional `paths` and `disable-model-invocation` frontmatter
// keys pass through when the spec declares them; arbitrary `x-cursor`
// keys pass through as well.
func emitSkill(s spec.Entry, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)

	if err := emit.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader(skillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
		return err
	}
	return emit.PropagateSkillAssets(s, folder, emit.SkipSKILLMd, dryRun)
}

func skillMarkdown(s spec.Entry) string {
	resolved := emit.ResolveMeta(s.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = s.Name
	}
	meta := map[string]any{
		"name":        s.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	for _, k := range []string{"paths", "disable-model-invocation"} {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	exclude := append([]string(nil), keys...)
	emit.MergeCustomTargetMeta(meta, &keys, s.Meta, target, exclude...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
