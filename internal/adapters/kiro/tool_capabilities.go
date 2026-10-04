package kiro

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

var toolCapabilities = emit.ToolCapabilityTable{
	"read":   {Agent: []string{"read"}},
	"Grep":   {Agent: []string{"read"}},
	"Glob":   {Agent: []string{"read"}},
	"write":  {Agent: []string{"write"}},
	"edit":   {Agent: []string{"write"}},
	"shell":  {Agent: []string{"shell"}},
	"web":    {Agent: []string{"web", "web"}},
	"delete": {Agent: []string{"write"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }
