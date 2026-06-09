package codex

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Codex CLI reads each generated
// artifact, honoring the same outputs.codex.* overrides Emit resolves.
// Rules are absent: Codex reads them through the entry-point pointer to
// the source specs, not from a native rules directory.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, defaultCommandsDir) + "/", Note: "prompts"},
		{Label: "Hooks", Location: emit.OutputHooksFile(cfg, target, defaultHooksFile)},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultConfigFile), Note: "mcp_servers tables"},
	}
}
