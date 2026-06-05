package amp

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSkill writes the native Amp skill layout:
//
//	<skillsDir>/<name>/
//	  SKILL.md         (required, frontmatter `name` + `description` + body)
//	  scripts/...      (any sibling files / subdirs from the source skill)
//
// Amp removed custom slash commands in favor of skills
// (https://ampcode.com/news/slashing-custom-commands); the current
// owner's manual reads skills from `.agents/skills/<name>/SKILL.md`,
// one folder per skill. The SKILL.md frontmatter is reduced to the two
// fields the manual documents so the file stays valid regardless of
// extra metadata the source carries; arbitrary `x-amp` keys still pass
// through.
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
	// Arbitrary x-amp keys pass through (sorted, target-scoped); the
	// hand-emitted name/description are excluded so nothing duplicates.
	emit.MergeCustomTargetMeta(meta, &keys, s.Meta, target, "name", "description")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
