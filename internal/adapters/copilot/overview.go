package copilot

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where GitHub Copilot reads each generated
// artifact, honoring the same outputs.copilot.* overrides Emit
// resolves. Chatmodes appear only when the user opted in via
// outputs.copilot.chatmodes-dir.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	arts := []emit.NativeArtifact{
		{Label: "Rules", Location: emit.OutputInstructionsDir(cfg, target, defaultInstructionsDir) + "/", Note: "path-scoped *.instructions.md"},
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/", Note: "one *.agent.md profile per agent"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/", Note: "one folder per skill"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
	if dir := emit.OutputChatmodesDir(cfg, target, ""); dir != "" {
		arts = append(arts, emit.NativeArtifact{Label: "Chat modes", Location: dir + "/"})
	}
	return arts
}
