package gemini

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Gemini CLI reads each generated
// artifact, honoring the same outputs.gemini.* overrides Emit resolves.
// Skills appear only with outputs.gemini.emit-skills-as-commands; rules
// reach Gemini through the entry-point pointer.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	arts := []emit.NativeArtifact{
		{Label: "Agents", Location: commandsDir + "/", Note: "one TOML per agent"},
		{Label: "Commands", Location: commandsDir + "/", Note: "one TOML per command"},
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		arts = append(arts, emit.NativeArtifact{Label: "Skills", Location: commandsDir + "/", Note: skillFilenamePrefix + "* commands"})
	}
	arts = append(arts, emit.NativeArtifact{
		Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultSettingsFile),
	})
	return arts
}
