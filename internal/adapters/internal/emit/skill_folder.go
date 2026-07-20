package emit

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SkillMarkdown renders the standard Agent Skills SKILL.md body: `name`
// + `description` frontmatter (description resolved through the
// per-target meta, falling back to the skill name), arbitrary
// `x-<target>` keys passed through, then the trimmed body. Extra
// exclude keys suppress custom-meta passthrough for fields the adapter
// routes elsewhere (e.g. codex's agents/openai.yaml keys).
//
// Every adapter of the shared SKILL.md format must render through this
// function: targets that emit into one tree (codex, amp, zed at
// `.agents/skills/`) rely on byte-identical output to dedupe, and
// `sync.shared-skills` links folders across trees only when the
// rendered bytes match.
func SkillMarkdown(s spec.Entry, target string, exclude ...string) string {
	resolved := ResolveMeta(s.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = s.Name
	}
	meta := map[string]any{
		"name":        s.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	MergeCustomTargetMeta(meta, &keys, s.Meta, target, append([]string{"name", "description"}, exclude...)...)
	front := FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// WriteSkillFolder writes the standard Agent Skills folder layout for
// one skill: `<skillsDir>/<name>/SKILL.md` rendered via SkillMarkdown
// plus every sibling asset propagated byte-for-byte. Adapters with
// extra per-skill artifacts (codex) or a wider frontmatter allowlist
// (cursor, claude) keep their own emit path.
func WriteSkillFolder(s spec.Entry, target, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)
	if err := WriteFile(filepath.Join(folder, "SKILL.md"), WithHeader(SkillMarkdown(s, target), FormatMarkdown), dryRun); err != nil {
		return err
	}
	return PropagateSkillAssets(s, folder, SkipSKILLMd, dryRun)
}

// WriteSkillFolders writes the standard Agent Skills folder layout for
// every skill into skillsDir. It is the multi-skill form of
// WriteSkillFolder, shared by the adapters that emit skills through the
// native folder layout with no per-target customization.
func WriteSkillFolders(skills []spec.Entry, target, skillsDir string, dryRun bool) error {
	for _, s := range skills {
		if err := WriteSkillFolder(s, target, skillsDir, dryRun); err != nil {
			return err
		}
	}
	return nil
}
