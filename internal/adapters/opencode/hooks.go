package opencode

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultPluginsDir is the project-level plugin directory OpenCode
// loads at startup: "Place JavaScript or TypeScript files in the plugin
// directory. `.opencode/plugins/` - Project-level plugins ... Files in
// these directories are automatically loaded at startup."
// (opencode.ai/docs/plugins/, re-read 2026-09-19).
const defaultPluginsDir = ".opencode/plugins"

// pluginHost adds OpenCode's TUI events to the shared bus vocabulary;
// Kilo does not list that group.
var pluginHost = func() emit.PluginHookHost {
	events := emit.PluginBusEvents()
	for _, e := range []string{"tui.prompt.append", "tui.command.execute", "tui.toast.show"} {
		events[e] = true
	}
	return emit.PluginHookHost{
		Target: target, Tool: "OpenCode", ImportModule: "@opencode-ai/plugin", BusEvents: events,
	}
}()

// emitHooks writes one plugin module per hook spec through the renderer
// OpenCode and Kilo share.
func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	return emit.EmitPluginHooks(sess, pluginHost, hooks, dir, dryRun)
}

// HookEvents lists every event a hook spec may name for this target, so
// `validate` reads the same vocabulary the emitter maps.
func HookEvents() []string {
	return pluginHost.Events()
}
