package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// clineRulesDirs lists the rules directories Cline reads, preferred
// first: the current config reference
// (docs.cline.bot/getting-started/config) lists `.cline/rules/` and
// does not mention `.clinerules/` anywhere, but the older
// docs.cline.bot/customization/cline-rules page still calls
// `.clinerules/` the "Primary rule format" (target-audit 2026-08-01,
// #534). Import walks the first one that exists so both pre- and
// post-migration projects round-trip. A pre-migration `.clinerules/`
// tree may still hold `agent-<name>.md` files (the old combined
// rules-and-agents convention); importRulesDirectory already
// reclassifies those by filename prefix, so they come back as agents
// exactly as they always did.
var clineRulesDirs = []string{
	filepath.Join(".cline", "rules"),
	".clinerules",
}

const (
	// clineAgentsDir is Cline's native per-agent directory
	// (docs.cline.bot/getting-started/config); files there are flat
	// `<name>.md`, not the pre-migration `agent-<name>.md` rule-form.
	clineAgentsDir = ".cline/agents"
)

// clineSkillsDirs lists every documented project skill path in precedence
// order. `.cline/skills/` is the recommended location, followed by the
// legacy `.clinerules/skills/` tree and Claude-compatible skills.
var clineSkillsDirs = []string{
	filepath.Join(".cline", "skills"),
	filepath.Join(".clinerules", "skills"),
	filepath.Join(".claude", "skills"),
}

// clineImportDir returns the first existing candidate rules dir under
// root, defaulting to the preferred `.cline/rules` when neither exists
// yet.
func clineImportDir(root string) string {
	for _, d := range clineRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return clineRulesDirs[0]
}

// importFromCline reads an existing Cline project and writes specs into
// the configured source directories, reversing the cline emit:
//
//   - `.cline/rules/*.md` (or the legacy `.clinerules/*.md`) walks via
//     the shared rules-directory importer. A `skill-<name>.md` there
//     still imports as a skill too, covering projects synced before
//     skills moved to a native folder; a legacy `agent-<name>.md`
//     there reclassifies as an agent, covering projects synced before
//     agents moved to their own directory (#534).
//   - `.cline/agents/*.md` (the native agents directory) reconstructs
//     agents, byte-for-byte minus the provenance header.
//   - `.cline/skills/`, `.clinerules/skills/`, and `.claude/skills/`
//     reconstruct native skill folders with bundled assets. Earlier paths
//     win same-name collisions.
func importFromCline(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	rulesDir := clineImportDir(root)
	opts := rulesDirImportOpts{}
	if rulesDir == ".clinerules" {
		opts.SkipDirs = map[string]bool{"skills": true}
	}
	c, err := importRulesDirectoryWith(root, rulesDir, src, opts)
	if err != nil {
		return err
	}
	nativeAgents, err := importFlatMarkdownFiles(filepath.Join(root, clineAgentsDir), filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	c.agents += nativeAgents
	seenSkills := map[string]bool{}
	for _, skillsDir := range clineSkillsDirs {
		folderSkills, err := importSkillFoldersWith(filepath.Join(root, skillsDir), filepath.Join(root, src.Skills), skillFolderImportOpts{SkipNames: seenSkills})
		if err != nil {
			return err
		}
		c.skills += folderSkills
	}
	summaryf("imported %d rules, %d agents, %d skills (from cline)\n", c.rules, c.agents, c.skills)
	printImportNextSteps(root, "cline")
	return nil
}
