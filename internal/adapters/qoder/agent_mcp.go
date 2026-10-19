package qoder

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentMCPForm reports that a subagent's `mcpServers` takes a
// "Server-name array, inline server object, or a mix"
// (docs.qoder.com/cli/subagent).
func (Adapter) AgentMCPForm() emit.AgentMCPForm {
	return emit.AgentMCPForm{Key: "mcpServers", Names: true}
}

// AgentMCPServers renders mcps in the settings.json server shape, whose
// fields the subagent page lists for inline servers.
func (Adapter) AgentMCPServers(mcps []spec.Entry) map[string]any {
	return buildMCPMap(emit.DropMCPWebSocket(target, mcps, mcpWebSocketGapReason))
}
