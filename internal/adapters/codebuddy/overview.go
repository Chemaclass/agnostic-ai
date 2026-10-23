package codebuddy

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	out := []emit.NativeArtifact{
		{Label: "Rules", Location: emit.OutputRulesDir(cfg, target, emit.OutputSubDir(cfg, target, "rules", ".codebuddy/rules")) + "/", Note: "conditional rules; always-on rules live in CODEBUDDY.md"},
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, emit.OutputSubDir(cfg, target, "agents", ".codebuddy/agents")) + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", ".codebuddy/skills")) + "/"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "commands", ".codebuddy/commands")) + "/"},
		{Label: "Settings and hooks", Location: emit.OutputConfFile(cfg, target, emit.OutputSubDir(cfg, target, "settings.json", ".codebuddy/settings.json"))},
	}
	if path := emit.OutputRootMCPFile(cfg, target); path != "" {
		out = append(out, emit.NativeArtifact{Label: "MCP servers", Location: path})
	}
	return out
}
