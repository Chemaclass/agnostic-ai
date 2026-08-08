package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// traeRulesDir is where Trae reads always-on rules and agents
	// (reclassified by filename prefix; see classifyRulesDirFile).
	traeRulesDir = ".trae/rules"
	// traeSkillsDir is Trae's native skills path
	// (docs.trae.ai/ide/skills): a folder per skill holding a SKILL.md,
	// the layout `sync` writes today.
	traeSkillsDir = ".trae/skills"
	// traeCommandsDir is where Trae reads custom commands: one markdown
	// file per command, `name` + `description` frontmatter and the body
	// as the prompt.
	traeCommandsDir = ".trae/commands"
	// traeMCPFile is the project-root MCP server registry Trae reads
	// (docs.trae.ai/ide/add-mcp-servers).
	traeMCPFile = ".trae/mcp.json"
	// traeMCPKey is the top-level JSON object holding the server map.
	traeMCPKey = "mcpServers"
)

// importFromTrae reads an existing Trae project and writes specs into
// the configured source directories, reversing the trae emit:
//
//   - `.trae/rules/*.md` reclassifies by filename prefix into rules and
//     agents (`agent-<name>.md`); a `skill-<name>.md` there still
//     imports as a skill too, covering projects synced before skills
//     moved to a native folder.
//   - `.trae/skills/<name>/SKILL.md` folders reconstruct skills
//     natively, with bundled sibling assets copied byte-for-byte.
//   - `.trae/commands/*.md` copies byte-for-byte into the commands
//     source dir.
//   - `.trae/mcp.json`'s `mcpServers` map writes one yaml per server.
//     Trae's schema has no `type` key, so a `url`-only entry infers
//     `type: http` on the way in (see importJSONMCPMap) the same way a
//     freshly emitted file would round-trip.
func importFromTrae(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, traeRulesDir, src)
	if err != nil {
		return err
	}
	folderSkills, err := importSkillFolders(filepath.Join(root, traeSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	c.skills += folderSkills
	commands, err := importTraeCommands(root, filepath.Join(root, src.Commands))
	if err != nil {
		return err
	}
	mcps, err := importTraeMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps (from trae)\n", c.rules, c.agents, c.skills, commands, mcps)
	printImportNextSteps(root, "trae")
	return nil
}

// importTraeCommands copies `.trae/commands/*.md` byte-for-byte into the
// commands source dir. Each command file becomes one command spec.
func importTraeCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, traeCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}

// importTraeMCP reads `.trae/mcp.json` and writes one yaml per
// `mcpServers.<name>` entry into dstDir. No-op when the file is absent.
func importTraeMCP(root, dstDir string) (int, error) {
	return importJSONMCPMap(filepath.Join(root, traeMCPFile), traeMCPKey, dstDir)
}
