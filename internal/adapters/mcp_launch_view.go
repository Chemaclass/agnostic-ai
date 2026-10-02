package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpLaunchViewer is an adapter whose MCP writer reads `url` or `args`
// from `x-<target>` too. Every other adapter writes the top-level ones.
type mcpLaunchViewer interface {
	MCPLaunchView() emit.MCPLaunchView
}

// rewriteMCPRefs writes mcps' references in a's own form, checking only
// the url and args a's MCP writer emits.
func rewriteMCPRefs(a Adapter, mcps []spec.Entry) []spec.Entry {
	var view emit.MCPLaunchView
	if v, ok := a.(mcpLaunchViewer); ok {
		view = v.MCPLaunchView()
	}
	return emit.RewriteMCPEnvRefs(a.Name(), view, mcps)
}
