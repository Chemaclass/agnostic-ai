package antigravity

import (
	"fmt"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// Both plugin MCP filenames retain their manifests.
var pluginComponents = map[string]bool{
	"skills": true, "agents": true, "rules": true,
	"mcp.json": true, "mcp_config.json": true, "hooks.json": true,
}

func notePluginMCPFilename(path string) {
	root := emit.PluginRootOf(path, pluginComponents)
	if root != "" && filepath.Base(path) == "mcp.json" {
		emit.NoteProject(fmt.Sprintf("antigravity: Antigravity documents plugin MCP in mcp_config.json; set outputs.antigravity.mcp-file to %s/mcp_config.json", root))
	}
}

// pluginRoots maps each configured output path to the plugin root it
// belongs to, so a bundle assembled from the per-kind output keys gets
// its manifest.
func pluginRoots(paths ...string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, emit.PluginRootOf(p, pluginComponents))
	}
	return out
}
