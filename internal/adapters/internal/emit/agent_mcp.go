package emit

// AgentMCPForm is how a target's agent file takes MCP servers.
type AgentMCPForm struct {
	// Key is the agent file key that holds the servers.
	Key string
	// Names is true when the agent's list can also name a server the
	// project MCP file defines. Such a target inlines only agent-only
	// servers and keeps every other entry a name.
	Names bool
}
