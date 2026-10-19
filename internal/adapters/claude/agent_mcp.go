package claude

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentMCPForm reports that an agent's `mcpServers` list mixes server
// names and inline definitions keyed by name
// (code.claude.com/docs/en/sub-agents). An inline server stays out of
// the main conversation.
func (Adapter) AgentMCPForm() emit.AgentMCPForm {
	return emit.AgentMCPForm{Key: "mcpServers", Names: true}
}

// AgentMCPServers renders mcps in the .mcp.json server shape, which
// inline agent servers share.
func (Adapter) AgentMCPServers(mcps []spec.Entry) map[string]any {
	mcps = emit.DropMCPDisabled(target, mcps, "a disabled agent-only server is left out of the agent file")
	return emit.BuildMCPServersMap(mcps, emit.MCPSchemaServersMap, emit.WithClaudeMCPExtras())
}
