package qwen

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, defaultCommandsDir) + "/"},
		{Label: "Settings", Location: emit.OutputConfFile(cfg, target, defaultSettingsFile)},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, emit.OutputConfFile(cfg, target, defaultSettingsFile))},
		{Label: "Hooks", Location: emit.OutputHooksFile(cfg, target, emit.OutputConfFile(cfg, target, defaultSettingsFile))},
		{Label: "Ignore", Location: emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile)},
	}
}
