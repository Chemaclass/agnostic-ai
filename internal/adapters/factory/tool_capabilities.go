package factory

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

var toolCapabilities = emit.ToolCapabilityTable{
	"read":       {Agent: []string{"Read"}},
	"LS":         {Agent: []string{"LS"}},
	"Grep":       {Agent: []string{"Grep"}},
	"Glob":       {Agent: []string{"Glob"}},
	"Create":     {Agent: []string{"Create"}},
	"edit":       {Agent: []string{"Edit"}},
	"ApplyPatch": {Agent: []string{"ApplyPatch"}},
	"Execute":    {Agent: []string{"Execute"}},
	"web":        {Agent: []string{"FetchUrl", "WebSearch"}},
	"FetchUrl":   {Agent: []string{"FetchUrl"}},
	"shell":      {Agent: []string{"Execute"}},
	"write":      {Agent: []string{"Create"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }
