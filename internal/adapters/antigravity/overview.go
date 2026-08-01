package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Antigravity reads each generated
// artifact, honoring the same outputs.antigravity.* overrides Emit
// resolves. Agents emit as agent-* files inside the rules directory.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	return []emit.NativeArtifact{
		{Label: "Rules", Location: rulesDir + "/", Note: "one file per rule"},
		{Label: "Agents", Location: rulesDir + "/", Note: "agent-* files"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
}
