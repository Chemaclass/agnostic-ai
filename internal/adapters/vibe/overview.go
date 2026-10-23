package vibe

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, ".vibe/agents") + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, ".vibe/skills") + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, emit.OutputConfFile(cfg, target, ".vibe/config.toml")), Note: "mcp_servers array"},
		{Label: "Hooks", Location: emit.OutputHooksFile(cfg, target, ".vibe/hooks.toml")},
	}
}
