package pi

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", ".pi/skills")) + "/"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "prompts", ".pi/prompts")) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, emit.OutputSubDir(cfg, target, "mcp.json", ".pi/mcp.json"))},
		{Label: "Settings", Location: emit.OutputConfFile(cfg, target, emit.OutputSubDir(cfg, target, "settings.json", ".pi/settings.json"))},
	}
}
