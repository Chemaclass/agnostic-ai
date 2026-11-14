package cli

import (
	"fmt"
	"strconv"
)

// openhandsNativeAgentFields moves the agent keys that have no shared spec
// field under x-openhands. Inline `mcp_servers` and `mcp_config` are
// committed with the spec, so one that holds a literal credential stops the
// import, as for Grok. A `${VAR}` or `$VAR` reference stays as written.
func openhandsNativeAgentFields(data []byte) ([]byte, error) {
	if mapping, _, ok := frontmatterNode(data); ok {
		var meta map[string]any
		if err := mapping.Decode(&meta); err == nil {
			for _, key := range []string{"mcp_servers", "mcp_config"} {
				if err := validateRetainedCredentials(openhandsInlineServers(meta[key])); err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
			}
		}
	}
	return importNativeAgentFields(data, "openhands", "skills", "tools", "permission_mode", "color", "hooks", "max_iteration_per_run", "max_budget_per_run", "condenser", "profile_store_dir", "mcp_config", "mcp_servers")
}

// openhandsInlineServers returns the servers of an agent's `mcp_servers`
// (a name-keyed map or a list) or `mcp_config` (which nests them under
// `mcpServers`) as the name-keyed map the credential check reads.
func openhandsInlineServers(value any) map[string]any {
	if list, ok := value.([]any); ok {
		servers := map[string]any{}
		for i, item := range list {
			servers[strconv.Itoa(i)] = item
		}
		return servers
	}
	servers, _ := value.(map[string]any)
	if nested, ok := servers["mcpServers"].(map[string]any); ok {
		return nested
	}
	return servers
}
