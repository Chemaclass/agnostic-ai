package warp

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Warp reads each generated artifact,
// honoring the same outputs.warp.* overrides Emit resolves. Workflows
// appear only when the user opted in via outputs.warp.workflows-dir;
// rules and skills reach Warp through the entry-point pointer.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	arts := []emit.NativeArtifact{
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
	if dir := emit.OutputWorkflowsDir(cfg, target, ""); dir != "" {
		arts = append(arts, emit.NativeArtifact{Label: "Agents", Location: dir + "/", Note: "Warp Workflows"})
	}
	return arts
}
