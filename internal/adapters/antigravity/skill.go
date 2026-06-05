package antigravity

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSkill writes the native Antigravity skill layout:
//
//	<skillsDir>/<name>/
//	  SKILL.md         (required, frontmatter `name` + `description` + body)
//	  <attached files> (any sibling files / subdirs from the source skill)
//
// Antigravity reads workspace skills from `.agent/skills/<name>/SKILL.md`
// (one folder per skill, mirroring the Codex layout). The SKILL.md
// frontmatter is reduced to the two fields the published schema needs so
// the file stays valid regardless of extra metadata the source carries.
func emitSkill(s spec.Entry, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)

	if err := emit.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader(skillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
		return err
	}
	return emit.PropagateSkillAssets(s, folder, skipSKILL, dryRun)
}

// skipSKILL excludes the re-rendered SKILL.md from sibling-asset
// propagation; everything else next to the source SKILL.md is copied
// verbatim.
func skipSKILL(rel string) bool { return rel == "SKILL.md" }

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
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
