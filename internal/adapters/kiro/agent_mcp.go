package kiro

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentMCPForm reports that an agent's `mcpServers` is a map of inline
// server definitions only (kiro.dev/docs/custom-agents/configuration-reference/).
func (Adapter) AgentMCPForm() emit.AgentMCPForm {
	return emit.AgentMCPForm{Key: "mcpServers"}
}

// AgentMCPServers renders mcps in the .kiro/settings/mcp.json server
// shape, which the agent field shares.
func (Adapter) AgentMCPServers(mcps []spec.Entry) map[string]any {
	mcps = emit.DropMCPDisabled(target, mcps, "a disabled server is left out of the agent file")
	return emit.BuildMCPServersMap(mcps, emit.MCPSchemaServersMap, emit.WithKiroMCPExtras())
}
