package codex

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Codex CLI reads each generated
// artifact, honoring the same outputs.codex.* overrides Emit resolves.
// Rules are absent: Codex reads them through the entry-point pointer to
// the source specs, not from a native rules directory. Commands appear
// only when outputs.codex.commands-dir opts into the deprecated
// project-level prompts layout.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	out := []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, defaultAgentsDir) + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
	}
	if commandsDir := emit.OutputCommandsDir(cfg, target, ""); commandsDir != "" {
		out = append(out, emit.NativeArtifact{Label: "Commands", Location: commandsDir + "/", Note: "prompts"})
	}
	return append(out,
		emit.NativeArtifact{Label: "Hooks", Location: emit.OutputHooksFile(cfg, target, defaultHooksFile)},
		emit.NativeArtifact{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultConfigFile), Note: "mcp_servers tables"},
	)
}
