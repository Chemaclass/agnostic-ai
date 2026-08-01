package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// clineRulesDir is where Cline reads always-on rules and agents
	// (reclassified by filename prefix; see classifyRulesDirFile).
	clineRulesDir = ".clinerules"
	// clineSkillsDir is Cline's recommended skills path
	// (docs.cline.bot/customization/skills): a folder per skill holding
	// a SKILL.md, the layout `sync` writes today.
	clineSkillsDir = ".cline/skills"
)

// importFromCline reads an existing Cline project and writes specs into
// the configured source directories, reversing the cline emit:
//
//   - `.clinerules/*.md` reclassifies by filename prefix into rules and
//     agents (`agent-<name>.md`); a `skill-<name>.md` there still
//     imports as a skill too, covering projects synced before skills
//     moved to a native folder.
//   - `.cline/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
func importFromCline(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, clineRulesDir, src)
	if err != nil {
		return err
	}
	folderSkills, err := importSkillFolders(filepath.Join(root, clineSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += folderSkills
	summaryf("imported %d rules, %d agents, %d skills (from cline)\n", c.rules, c.agents, c.skills)
	printImportNextSteps(root, "cline")
	return nil
}
