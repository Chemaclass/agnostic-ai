package spec

// AgentOnlyKey marks an MCP spec that only the agents naming it may
// reach: sync keeps it out of every project and user MCP file.
const AgentOnlyKey = "agent-only"

// AgentOnly reports whether e is an MCP spec marked `agent-only: true`.
func (e Entry) AgentOnly() bool {
	only, _ := e.Meta[AgentOnlyKey].(bool)
	return e.Kind == KindMCP && only
}

// SplitAgentOnly separates the MCP specs that project and user MCP files
// list from the agent-only ones.
func SplitAgentOnly(mcps []Entry) (shared, agentOnly []Entry) {
	for _, e := range mcps {
		if e.AgentOnly() {
			agentOnly = append(agentOnly, e)
		} else {
			shared = append(shared, e)
		}
	}
	return shared, agentOnly
}

// AgentMCPNames returns the server names in an agent's portable
// `mcpServers` list, skipping any entry that is not a name.
func AgentMCPNames(meta map[string]any) []string {
	list, _ := meta["mcpServers"].([]any)
	var names []string
	for _, item := range list {
		if name, ok := item.(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}
