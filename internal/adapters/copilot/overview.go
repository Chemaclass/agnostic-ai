package copilot

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where GitHub Copilot reads each generated
// artifact, honoring the same outputs.copilot.* overrides Emit
// resolves. Agents and skills emit into the instructions directory with
// agent- / skill- filename prefixes; chatmodes appear only when the
// user opted in via outputs.copilot.chatmodes-dir.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	instructionsDir := emit.OutputInstructionsDir(cfg, target, defaultInstructionsDir)
	arts := []emit.NativeArtifact{
		{Label: "Rules", Location: instructionsDir + "/", Note: "path-scoped *.instructions.md"},
		{Label: "Agents", Location: instructionsDir + "/", Note: "agent-* files"},
		{Label: "Skills", Location: instructionsDir + "/", Note: "skill-* files"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
	if dir := emit.OutputChatmodesDir(cfg, target, ""); dir != "" {
		arts = append(arts, emit.NativeArtifact{Label: "Chat modes", Location: dir + "/"})
	}
	return arts
}
