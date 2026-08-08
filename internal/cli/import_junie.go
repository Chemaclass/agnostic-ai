package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// junieEntryPointFile is the file Junie's strict-precedence
	// guidelines lookup always finds first in a synced project (#552):
	// rule and agent bodies inline here under their own sentinel-marked
	// blocks (see junie.go's emitEntryPoint), so it is the primary
	// source for both.
	junieEntryPointFile = ".junie/AGENTS.md"
	// junieLegacyRulesDir is the pre-#552 flattened rules-and-agents
	// directory (reclassified by filename prefix; see
	// classifyRulesDirFile). `sync` no longer writes it, but a project
	// synced by an older agnostic-ai version still has real content
	// there, so it stays the preferred import source when present.
	junieLegacyRulesDir = ".junie/rules"
	// junieSkillsDir is Junie's native Agent Skills folder tree
	// (junie.jetbrains.com/docs/agent-skills.html, target-audit
	// 2026-08-01): a folder per skill holding a SKILL.md, the layout
	// `sync` writes today.
	junieSkillsDir = ".junie/skills"
)

// importFromJunie reads an existing Junie project and writes specs into
// the configured source directories, reversing the junie emit:
//
//   - `.junie/AGENTS.md`'s sentinel-marked Rules and Agents blocks
//     reconstruct rule and agent specs. #552 established this is the
//     only file Junie's guidelines lookup ever opens in a synced
//     project, so it is the primary source for both kinds. When
//     `.junie/rules/` still exists (a project synced before that fix),
//     it takes precedence instead: reclassified by filename prefix
//     (`agent-<name>.md` → agent), the pre-#552 layout. A
//     `skill-<name>.md` there still imports as a skill too, covering
//     projects synced before skills moved to a native folder.
//   - `.junie/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
func importFromJunie(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importJunieRulesAndAgents(root, src)
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

// importJunieRulesAndAgents reconstructs rule and agent specs, preferring
// the pre-#552 flattened `.junie/rules/` layout when it still exists on
// disk (an older agnostic-ai version wrote real content there, and the
// new .junie/AGENTS.md blocks would be empty or absent) and otherwise
// reading .junie/AGENTS.md's sentinel-marked Rules and Agents blocks,
// the file junie.go's emitEntryPoint writes today (see its package doc).
func importJunieRulesAndAgents(root string, src config.Sources) (rulesDirCounts, error) {
	if dirExists(filepath.Join(root, junieLegacyRulesDir)) {
		return importRulesDirectory(root, junieLegacyRulesDir, src)
	}

	var c rulesDirCounts
	path := filepath.Join(root, junieEntryPointFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", path, err)
	}
	body := string(data)

	rules, err := importJunieAppendix(body, adapters.RulesStartMarker, adapters.RulesEndMarker, filepath.Join(root, src.Rules))
	if err != nil {
		return c, err
	}
	c.rules = rules

	agents, err := importJunieAppendix(body, adapters.AgentsStartMarker, adapters.AgentsEndMarker, filepath.Join(root, src.Agents))
	if err != nil {
		return c, err
	}
	c.agents = agents

	return c, nil
}

// importJunieAppendix extracts the sentinel-marked block bounded by
// start/end from body, unwraps its `### <name>` children (the same
// WriteSection shape RenderRulesAppendix and RenderAgentsAppendix both
// produce), and writes one rule-shaped spec file per child into dstDir.
// Returns 0 with no error when the markers are absent (a bundle with no
// rules, or none with agents, renders no block at all) or the block has
// no H3 children.
func importJunieAppendix(body, start, end, dstDir string) (int, error) {
	inner := extractMarkedBlock(body, start, end)
	if inner == "" {
		return 0, nil
	}
	children, ok := unwrapMergedH3Children(inner, map[string]int{})
	if !ok {
		return 0, nil
	}
	for _, c := range children {
		path := filepath.Join(dstDir, c.slug+".md")
		if err := writeRule(path, c.slug, c.body); err != nil {
			return 0, err
		}
	}
	return len(children), nil
}
