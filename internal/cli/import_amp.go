package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	ampMainFile     = "AGENTS.md"
	ampCommandsDir  = ".agents/commands"
	ampSettingsKey  = "amp.mcpServers"
	ampSettingsFile = ".amp/settings.json"
)

// importFromAmp reads an existing Sourcegraph Amp project (AGENTS.md,
// `.agents/commands/`, `.amp/settings.json`) under root and writes
// specs into the configured source directories.
func importFromAmp(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.MCPs); err != nil {
		return err
	}
	rules, err := importAmpRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importAmpCommands(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, err := importAmpMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if err := mirrorMainFile(root, ampMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d mcps\n", rules, agents, mcps)
	printImportNextSteps(root, "amp")
	return nil
}

// importAmpRules slices AGENTS.md by `## ` headings. Amp has no native
// rules directory, so the main file is always the source.
func importAmpRules(root, dstDir string) (int, error) {
	return sliceMainFileByH2(root, ampMainFile, dstDir)
}

// importAmpCommands copies `.agents/commands/*.md` byte-for-byte into
// the agents source dir. Each command file becomes one agent.
func importAmpCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, ampCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}

// importAmpMCP reads `.amp/settings.json` and writes one yaml per
// `amp.mcpServers.<name>` entry into <dstDir>/<name>.yaml.
func importAmpMCP(root, dstDir string) (int, error) {
	return importJSONMCPMap(filepath.Join(root, ampSettingsFile), ampSettingsKey, dstDir)
}
