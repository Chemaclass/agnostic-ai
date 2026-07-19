package copilot

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSkill writes the native Copilot skill layout:
//
//	<skillsDir>/<name>/
//	  SKILL.md         (required, frontmatter `name` + `description` + body)
//	  <attached files> (any sibling files / subdirs from the source skill)
//
// Copilot discovers repository skills at `.github/skills/<name>/SKILL.md`
// (it also scans the cross-tool `.claude/skills/` and `.agents/skills/`
// paths). Arbitrary `x-copilot` keys (license, allowed-tools, ...) pass
// through the frontmatter.
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
	emit.MergeCustomTargetMeta(meta, &keys, s.Meta, target, "name", "description")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
