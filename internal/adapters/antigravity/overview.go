package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Antigravity reads each generated
// artifact, honoring the same outputs.antigravity.* overrides Emit
// resolves.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	return []emit.NativeArtifact{
		{Label: "Rules", Location: rulesDir + "/", Note: "one file per rule"},
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/", Note: "one <name>/agent.md profile per subagent"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
}
