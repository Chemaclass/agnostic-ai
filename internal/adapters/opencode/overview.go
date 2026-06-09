package opencode

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where OpenCode reads each generated
// artifact, honoring the same outputs.opencode.* overrides Emit
// resolves. Skills appear only with
// outputs.opencode.emit-skills-as-commands; rules reach OpenCode
// through the entry-point pointer.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	arts := []emit.NativeArtifact{
		{Label: "Agents", Location: commandsDir + "/", Note: "slash commands"},
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		arts = append(arts, emit.NativeArtifact{Label: "Skills", Location: commandsDir + "/", Note: skillFilenamePrefix + "* commands"})
	}
	arts = append(arts, emit.NativeArtifact{
		Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile),
	})
	return arts
}
