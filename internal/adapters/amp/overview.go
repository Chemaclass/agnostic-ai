package amp

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Amp reads each generated artifact,
// honoring the same outputs.amp.* overrides Emit resolves. Rules are
// absent: Amp reads them through the entry-point pointer to the source
// specs.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputCommandsDir(cfg, target, defaultCommandsDir) + "/", Note: "slash commands"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, defaultCommandsDir) + "/", Note: "slash commands"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
}
