package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// windsurfRulesDirs lists the rules directories Devin Desktop (the
// renamed Windsurf) reads, preferred first. Import walks the first one
// that exists so both pre- and post-rename projects round-trip.
var windsurfRulesDirs = []string{
	filepath.Join(".devin", "rules"),
	filepath.Join(".windsurf", "rules"),
}

// windsurfSkillsDir is the shared cross-tool skills tree Devin Desktop
// scans (docs.devin.ai/desktop/cascade/skills): a folder per skill
// holding a SKILL.md, the same tree codex, amp, zed, crush, and
// openhands emit into.
const windsurfSkillsDir = ".agents/skills"

// windsurfImportDir returns the first existing candidate rules dir
// under root, defaulting to the preferred `.devin/rules` when neither
// exists yet.
func windsurfImportDir(root string) string {
	for _, d := range windsurfRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return windsurfRulesDirs[0]
}

// importFromWindsurf reads an existing Devin Desktop / Windsurf project
// and writes specs into the configured source directories, reversing
// the windsurf emit:
//
//   - `.devin/rules/*.md` (or the legacy `.windsurf/rules/*.md`)
//     reclassifies by filename prefix into rules and agents
//     (`agent-<name>.md`); a `skill-<name>.md` there still imports as a
//     skill too, covering projects synced before skills moved to a
//     native folder.
//   - `.agents/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
func importFromWindsurf(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, windsurfImportDir(root), src)
	if err != nil {
		return err
	}
	folderSkills, err := importSkillFolders(filepath.Join(root, windsurfSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += folderSkills
	summaryf("imported %d rules, %d agents, %d skills (from windsurf)\n", c.rules, c.agents, c.skills)
	printImportNextSteps(root, "windsurf")
	return nil
}
