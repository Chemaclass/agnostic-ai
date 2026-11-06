package kiro

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentMCPForm reports that an agent's `mcpServers` is a map of inline
// server definitions only (kiro.dev/docs/custom-agents/configuration-reference/).
func (Adapter) AgentMCPForm() emit.AgentMCPForm {
	return emit.AgentMCPForm{Key: "mcpServers"}
}

// agentMCPTimeoutKeys are the per-server timeouts, in milliseconds, that
// Kiro documents on agent `mcpServers` entries only: `timeout` for the
// connection handshake and `requestTimeout` per call. Its
// `.kiro/settings/mcp.json` lists neither.
var agentMCPTimeoutKeys = []string{"timeout", "requestTimeout"}

// AgentMCPServers renders mcps in the .kiro/settings/mcp.json server
// shape, plus the agent-only timeouts set under `x-kiro`. The portable
// `timeout` is not written: Kiro splits it into two limits, and the spec
// does not say which one it means.
func (Adapter) AgentMCPServers(mcps []spec.Entry) map[string]any {
	mcps = emit.DropMCPDisabled(target, mcps, "a disabled server is left out of the agent file")
	servers := emit.BuildMCPServersMap(mcps, emit.MCPSchemaServersMap, emit.WithKiroMCPExtras())
	for _, e := range mcps {
		server, ok := servers[e.Name].(map[string]any)
		if !ok {
			continue
		}
		custom, _ := e.Meta[emit.XPrefix+target].(map[string]any)
		for _, key := range agentMCPTimeoutKeys {
			raw, set := custom[key]
			if !set || raw == nil {
				continue
			}
			ms, whole := emit.WholeNumber(raw)
			if !whole || ms <= 0 {
				emit.NoteProject(fmt.Sprintf("%s: MCP %s x-%s.%s %v is not a positive whole number of milliseconds, so it is not written to agent files", target, e.Name, target, key, raw))
				continue
			}
			server[key] = ms
		}
		_, native := custom["timeout"]
		_, perCall := custom["requestTimeout"]
		// One line per server: notes repeat per agent, and identical
		// lines print once.
		if _, set := e.Meta["timeout"]; set && !native && !perCall {
			emit.NoteProject(fmt.Sprintf("%s: MCP %s timeout is not written to agent files, since Kiro splits it into a handshake and a per-call limit; set x-kiro.timeout or x-kiro.requestTimeout in milliseconds", target, e.Name))
		}
	}
	return servers
}
