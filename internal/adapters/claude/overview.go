package claude

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Claude Code reads each generated
// artifact, honoring the same outputs.claude.* overrides Emit resolves.
// Consumed by the sync layer to render the target-overview appendix in
// CLAUDE.md when sync.target-overview is enabled.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	dir := emit.OutputDir(cfg, target, defaultDir)
	return []emit.NativeArtifact{
		{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, emit.OutputSubDir(cfg, target, "agents", defaultAgentsDir)) + "/"},
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", defaultSkillsDir)) + "/"},
		{Label: "Rules", Location: emit.OutputRulesDir(cfg, target, emit.OutputSubDir(cfg, target, "rules", defaultRulesDir)) + "/", Note: "one file per rule"},
		{Label: "Commands", Location: emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "commands", defaultCommandsDir)) + "/"},
		{Label: "Hooks", Location: filepath.Join(dir, "settings.json"), Note: "hooks key"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
}
