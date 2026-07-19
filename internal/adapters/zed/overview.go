package zed

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Zed reads each generated artifact,
// honoring the same outputs.zed.* overrides Emit resolves. Rules reach
// Zed through the shared AGENTS.md entry-point; tasks appear only when
// the user opted in via outputs.zed.tasks-file.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	arts := []emit.NativeArtifact{
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/", Note: "one folder per skill"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile), Note: "context_servers"},
	}
	if f := emit.OutputTasksFile(cfg, target, ""); f != "" {
		arts = append(arts, emit.NativeArtifact{Label: "Tasks", Location: f, Note: "one task per hook"})
	}
	return arts
}
