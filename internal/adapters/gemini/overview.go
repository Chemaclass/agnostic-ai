package gemini

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Gemini CLI reads each generated
// artifact, honoring the same outputs.gemini.* overrides Emit resolves.
// Rules reach Gemini through the entry-point pointer.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	arts := []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/", Note: "one subagent per agent"},
		{Label: "Commands", Location: commandsDir + "/", Note: "one TOML per command"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/", Note: "one folder per skill"},
	}
	if emit.EmitAgentsAsCommands(cfg, target) {
		arts = append(arts, emit.NativeArtifact{Label: "Agent commands", Location: commandsDir + "/", Note: "one TOML per agent"})
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		arts = append(arts, emit.NativeArtifact{Label: "Skill commands", Location: commandsDir + "/", Note: skillFilenamePrefix + "* commands"})
	}
	arts = append(arts, emit.NativeArtifact{
		Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultSettingsFile),
	})
	return arts
}
