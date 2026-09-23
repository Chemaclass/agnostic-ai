package kilo

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultPluginsDir is the project-level plugin directory Kilo Code
// loads at startup: "Drop TypeScript or JavaScript files into a
// `plugin/` or `plugins/` folder inside any config directory ...
// Project: `.kilo/plugin/` or legacy `.kilocode/plugin/`" and "Every
// `.ts` or `.js` file in those directories is auto-registered at
// startup" (kilo.ai/docs/automate/extending/plugins, raw
// packages/kilo-docs/pages/automate/extending/plugins.md, read
// 2026-09-23). This adapter writes only the current `.kilo/plugin/`
// path, not the legacy `.kilocode/plugin/` one, the same choice this
// package already makes for rules and skills (see the package doc).
const defaultPluginsDir = ".kilo/plugin"

// pluginHost renders Kilo's module shape: "Plugins must default-export
// a module descriptor. `id` is required for local-file plugins", as in
// `export default { id: "my-plugin", server }`.
var pluginHost = emit.PluginHookHost{
	Target: target, Tool: "Kilo", ImportModule: "@kilocode/plugin",
	DefaultExport: true, BusEvents: emit.PluginBusEvents(),
}

// emitHooks writes one plugin module per hook spec through the renderer
// OpenCode and Kilo share.
func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	return emit.EmitPluginHooks(sess, pluginHost, hooks, dir, dryRun)
}
