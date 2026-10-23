package grok

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable {
	return emit.ToolCapabilityTable{"read": {Agent: []string{"Read"}, Permission: []string{"Read"}}, "write": {Agent: []string{"Write"}, Permission: []string{"Write"}}, "edit": {Agent: []string{"Edit"}, Permission: []string{"Edit"}}, "shell": {Agent: []string{"Bash"}, Permission: []string{"Bash"}}, "web": {Agent: []string{"WebFetch", "WebSearch"}, Permission: []string{"WebFetch", "WebSearch"}}}
}
