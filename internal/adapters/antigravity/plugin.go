package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// pluginComponents are the component names an Antigravity plugin
// directory may carry beside its manifest: "mcp_config.json",
// "hooks.json", "skills/", "agents/", and "rules/"
// (antigravity.google/docs/plugins). Anything else under a plugin root
// is not a component the tool reads, so it must not imply a plugin.
var pluginComponents = map[string]bool{
	"skills": true, "agents": true, "rules": true,
	"mcp_config.json": true, "hooks.json": true,
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
