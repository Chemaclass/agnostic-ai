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
	// rule bodies inline here under their own sentinel-marked block
	// (see junie.go's emitEntryPoint), so it is the primary source for
	// rules. It also carries the pre-#604 sentinel-marked Agents block
	// for a project synced by an adapter version between #552 and #604,
	// still read as a fallback when no native agent file exists (see
	// importJunieAgents).
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
	// junieCommandsDir is Junie's native slash-command folder
	// (junie.jetbrains.com/docs/custom-slash-commands.html, target-audit
	// 2026-08-11, #605): one `.md` per command, `description` the only
	// vendor-documented frontmatter field.
	junieCommandsDir = ".junie/commands"
)

// junieAgentsDirs lists the directories junie-cli-subagents.html
// documents Junie CLI scanning for native subagent files at project
// scope (junie.jetbrains.com/docs/junie-cli-subagents.html, target-audit
// 2026-08-11, #604): `.junie/agents/`, this adapter's own default, and
// `.agents/`, the vendor's shared alternative reachable via
// outputs.junie.agents-dir. The native path comes first so a project
// carrying both prefers the one this adapter writes.
var junieAgentsDirs = []string{".junie/agents", ".agents"}

// importFromJunie reads an existing Junie project and writes specs into
// the configured source directories, reversing the junie emit:
//
//   - `.junie/AGENTS.md`'s sentinel-marked Rules block reconstructs
//     rule specs. #552 established this is the only file Junie's
//     guidelines lookup ever opens in a synced project, so it is the
//     primary source. When `.junie/rules/` still exists (a project
//     synced before that fix), it takes precedence instead for both
//     rules and agents: reclassified by filename prefix
//     (`agent-<name>.md` → agent), the pre-#552 layout. A
//     `skill-<name>.md` there still imports as a skill too, covering
//     projects synced before skills moved to a native folder.
//   - `.junie/agents/<name>.md` (or `.agents/<name>.md`) files
//     reconstruct agent specs natively (#604). A project synced between
//     #552 and #604 has no native agent file yet; its agent bodies
//     still sit in `.junie/AGENTS.md`'s sentinel-marked Agents block,
//     read as a fallback when no native file is found.
//   - `.junie/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
//   - `.junie/commands/<name>.md` files reconstruct command specs
//     natively (#605).
func importFromJunie(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands); err != nil {
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
	commands, err := importJunieCommands(root, filepath.Join(root, src.Commands))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands (from junie)\n", c.rules, c.agents, c.skills, commands)
	printImportNextSteps(root, "junie")
	return nil
}

// importJunieRulesAndAgents reconstructs rule and agent specs, preferring
// the pre-#552 flattened `.junie/rules/` layout when it still exists on
// disk (an older agnostic-ai version wrote real content there, and
// agents never had a native destination yet either) and otherwise
// reading rules from `.junie/AGENTS.md`'s sentinel-marked Rules block
// and agents from their native file (falling back to that same file's
// pre-#604 Agents block; see importJunieAgents).
func importJunieRulesAndAgents(root string, src config.Sources) (rulesDirCounts, error) {
	if dirExists(filepath.Join(root, junieLegacyRulesDir)) {
		return importRulesDirectory(root, junieLegacyRulesDir, src)
	}

	var c rulesDirCounts
	agents, err := importJunieAgents(root, filepath.Join(root, src.Agents))
	if err != nil {
		return c, err
	}
	c.agents = agents

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

	if c.agents == 0 {
		legacyAgents, err := importJunieAppendix(body, adapters.AgentsStartMarker, adapters.AgentsEndMarker, filepath.Join(root, src.Agents))
		if err != nil {
			return c, err
		}
		c.agents = legacyAgents
	}

	return c, nil
}

// importJunieAgents copies every `*.md` file from the first directory
// in junieAgentsDirs that exists into dstDir, stripping the
// agnostic-ai provenance header the same way importClaudeAgents does.
// Returns 0 with no error when neither native directory exists, so the
// caller falls back to the pre-#604 Agents appendix in
// `.junie/AGENTS.md`.
func importJunieAgents(root, dstDir string) (int, error) {
	for _, sub := range junieAgentsDirs {
		src := filepath.Join(root, sub)
		if !dirExists(src) {
			continue
		}
		return copyMarkdownDir(src, dstDir)
	}
	return 0, nil
}

// importJunieCommands copies `.junie/commands/*.md` byte-for-byte into
// the commands source dir (#605). The agnostic-ai provenance header is
// stripped, but every frontmatter key round-trips verbatim, the same
// way importClaudeCommands handles Claude Code's own command folder.
func importJunieCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, junieCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
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
