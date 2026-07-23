package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	crushMainFile  = "AGENTS.md"
	crushSkillsDir = ".agents/skills"
	crushMCPFile   = "crush.json"
	crushMCPKey    = "mcp"
)

// importFromCrush reads an existing Charm Crush project and writes specs
// into the configured source directories, reversing the crush emit:
//
//   - `AGENTS.md` carries rule bodies inlined in a sentinel-marked
//     `## Rules` block (Crush has no per-rule directory); each `### <name>`
//     child becomes a rule. The same file mirrors to
//     `.agnostic-ai/AGNOSTIC_AI.md`.
//   - `.agents/skills/<name>/SKILL.md` folders reconstruct skills, with
//     bundled sibling assets copied byte-for-byte.
//   - `crush.json` (`mcp` map) reconstructs MCP specs, both `type: stdio`
//     and `type: http` entries; user-managed keys (models, providers,
//     lsp, options) are ignored.
//
// Lossy field: rules reach Crush only through the inlined block, which
// carries no `globs`/scope, so rule scoping does not round-trip (Crush's
// output is unaffected either way).
func importFromCrush(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Skills, src.MCPs); err != nil {
		return err
	}
	rules, err := sliceMainFileByH2(root, crushMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(filepath.Join(root, crushSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap(filepath.Join(root, crushMCPFile), crushMCPKey,
		filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, crushMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d skills, %d mcps\n", rules, skills, mcps)
	printImportNextSteps(root, "crush")
	return nil
}
