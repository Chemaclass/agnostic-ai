package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// Plugins bundle MCP servers in mcp.json; mcp_config.json is the
// standalone file, which the plugin layout does not list
// (antigravity.google/docs/build-plugins#supported-components).
var pluginComponents = map[string]bool{
	"skills": true, "agents": true, "rules": true,
	"mcp.json": true, "hooks.json": true,
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
