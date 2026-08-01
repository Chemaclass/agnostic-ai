package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// junieRulesDir is where Junie reads always-on rules and agents
	// (reclassified by filename prefix; see classifyRulesDirFile).
	junieRulesDir = ".junie/rules"
	// junieSkillsDir is Junie's native Agent Skills folder tree
	// (junie.jetbrains.com/docs/agent-skills.html, target-audit
	// 2026-08-01): a folder per skill holding a SKILL.md, the layout
	// `sync` writes today.
	junieSkillsDir = ".junie/skills"
)

// importFromJunie reads an existing Junie project and writes specs into
// the configured source directories, reversing the junie emit:
//
//   - `.junie/rules/*.md` reclassifies by filename prefix into rules and
//     agents (`agent-<name>.md`); a `skill-<name>.md` there still
//     imports as a skill too, covering projects synced before skills
//     moved to a native folder.
//   - `.junie/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
func importFromJunie(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, junieRulesDir, src)
	if err != nil {
		return err
	}
	folderSkills, err := importSkillFolders(filepath.Join(root, junieSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += folderSkills
	summaryf("imported %d rules, %d agents, %d skills (from junie)\n", c.rules, c.agents, c.skills)
	printImportNextSteps(root, "junie")
	return nil
}
