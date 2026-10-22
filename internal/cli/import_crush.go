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
//     bundled sibling assets copied byte-for-byte. See normalizeCrushSkill
//     for the user-only fields.
//   - `crush.json` (`mcp` map) reconstructs MCP specs: `type: stdio`,
//     `type: http`, and `type: sse` entries all pass through verbatim
//     via the shared JSON-map importer; user-managed keys (models,
//     providers, lsp, options) are ignored.
//   - `crush.json` (`hooks.PreToolUse` array) reconstructs hook specs,
//     the only event Crush's own runtime consumes. See
//     import_crush_hooks.go.
//
// Lossy field: rules reach Crush only through the inlined block, which
// carries no `globs`/scope, so rule scoping does not round-trip (Crush's
// output is unaffected either way).
func importFromCrush(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Skills, src.Hooks, src.MCPs); err != nil {
		return err
	}
	rules, err := sliceMirroredMainFile(root, crushMainFile, importSourcePath(root, src.Rules))
	if err != nil {
		return err
	}
	skills, err := importSkillFoldersWith(root, filepath.Join(root, crushSkillsDir), importSourcePath(root, src.Skills), skillFolderImportOpts{TransformSkill: normalizeCrushSkill})
	if err != nil {
		return err
	}
	hooks, err := importCrushHooks(root, importSourcePath(root, src.Hooks))
	if err != nil {
		return err
	}
	mcps, err := importJSONMCPMap("crush", filepath.Join(root, crushMCPFile), crushMCPKey,
		importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, crushMainFile); err != nil {
		return err
	}
	ignores, err := importIgnoreFile(root, "crush", src)
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d skills, %d hooks, %d mcps, %d ignores\n", rules, skills, hooks, mcps, ignores)
	printImportNextSteps(root, "crush")
	return nil
}

// normalizeCrushSkill turns `disable-model-invocation: true` with
// `user-invocable: true`, the pair sync writes for a user-only skill,
// back into the portable field. Either key alone, or a false one, means
// something only Crush reads, so it moves under x-crush.
func normalizeCrushSkill(data []byte) ([]byte, error) {
	meta, body := splitMdcFrontmatter(data)
	manual, hasManual := meta["disable-model-invocation"]
	invocable, hasInvocable := meta["user-invocable"]
	switch {
	case !hasManual && !hasInvocable:
		return data, nil
	case manual == true && invocable == true:
		delete(meta, "user-invocable")
		return skillMarkdownOf(meta, body)
	}
	targetMeta, _ := meta["x-crush"].(map[string]any)
	if targetMeta == nil {
		targetMeta = map[string]any{}
	}
	for key, value := range map[string]any{"disable-model-invocation": manual, "user-invocable": invocable} {
		if _, ok := meta[key]; ok {
			targetMeta[key] = value
			delete(meta, key)
		}
	}
	// Crush leaves a skill out of the command palette unless it sets
	// user-invocable, and sync would add it to a user-only skill.
	if manual == true && !hasInvocable {
		targetMeta["user-invocable"] = false
	}
	meta["x-crush"] = targetMeta
	return skillMarkdownOf(meta, body)
}
