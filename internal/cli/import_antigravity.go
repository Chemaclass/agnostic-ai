package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// antigravityRulesDirs lists the rules directories Antigravity reads,
// preferred first: "Antigravity now defaults to `.agents/rules`, but
// still maintains backward support for `.agent/rules`"
// (antigravity.google/docs/rules-workflows). Import walks the first one
// that exists so both pre- and post-plural-default projects round-trip.
var antigravityRulesDirs = []string{
	filepath.Join(".agents", "rules"),
	filepath.Join(".agent", "rules"),
}

const antigravityMainFile = ".agent/AGENTS.md"

// antigravityImportDir returns the first existing candidate rules dir
// under root, defaulting to the preferred `.agents/rules` when neither
// exists yet.
func antigravityImportDir(root string) string {
	for _, d := range antigravityRulesDirs {
		if dirExists(filepath.Join(root, d)) {
			return d
		}
	}
	return antigravityRulesDirs[0]
}

// importFromAntigravity reads an existing Antigravity project under
// root and writes specs into the configured source directories.
//
//   - `.agents/rules/*.md` (or the legacy `.agent/rules/*.md`) walks via
//     the shared rules-directory importer (agent-<name>.md routes to
//     agents, the rest to rules; the provenance header and the leading
//     `# <heading>\n` block are stripped from each body).
//   - When `outputs.antigravity.rules-file` is set in agnostic-ai.yaml,
//     the legacy concatenated file is sliced by H2 sections.
//   - `.agent/AGENTS.md` mirrors into `.agnostic-ai/AGNOSTIC_AI.md`
//     when present so a hand-edit propagates back into the source body.
func importFromAntigravity(root string, src config.Sources, cfg *config.Config) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills); err != nil {
		return err
	}
	c, err := importRulesDirectory(root, antigravityImportDir(root), src)
	if err != nil {
		return err
	}

	rulesFileCount := 0
	if rulesFile := antigravityRulesFileFromCfg(cfg); rulesFile != "" {
		n, err := sliceMainFileByH2(root, rulesFile, filepath.Join(root, src.Rules))
		if err != nil {
			return err
		}
		rulesFileCount = n
	}

	if _, err := mirrorMainFile(root, antigravityMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills\n",
		c.rules+rulesFileCount, c.agents, c.skills)
	printImportNextSteps(root, "antigravity")
	return nil
}

// antigravityRulesFileFromCfg returns the project-relative
// `outputs.antigravity.rules-file` path when configured, otherwise "".
func antigravityRulesFileFromCfg(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if o, ok := cfg.Outputs["antigravity"]; ok {
		return o.RulesFile
	}
	return ""
}
