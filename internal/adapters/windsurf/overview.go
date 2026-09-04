package windsurf

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Devin Desktop and Devin CLI read each
// generated artifact, honoring the same outputs.windsurf.* overrides
// Emit resolves.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Rules", Location: emit.OutputRulesDir(cfg, target, defaultDir) + "/", Note: "one file per rule"},
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/", Note: "custom subagent profiles"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
		{Label: "Ignore", Location: emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile)},
	}
}
