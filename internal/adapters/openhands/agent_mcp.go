package openhands

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentMCPForm reports that a file-based agent's `mcp_servers` is a map
// of inline definitions "in the same format as the MCP configuration"
// (docs.openhands.dev/sdk/guides/agent-file-based).
func (Adapter) AgentMCPForm() emit.AgentMCPForm {
	return emit.AgentMCPForm{Key: "mcp_servers"}
}

// AgentMCPServers renders mcps in the OpenHands MCP config shape.
func (a Adapter) AgentMCPServers(mcps []spec.Entry) map[string]any {
	return a.UserMCPServers(mcps)
}
