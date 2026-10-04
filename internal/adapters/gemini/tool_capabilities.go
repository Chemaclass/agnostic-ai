package gemini

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

var toolCapabilities = emit.ToolCapabilityTable{
	"read":  {Agent: []string{"read_file"}},
	"write": {Agent: []string{"write_file"}},
	"edit":  {Agent: []string{"replace"}},
	"Glob":  {Agent: []string{"glob"}},
	"Grep":  {Agent: []string{"grep_search"}},
	"shell": {Agent: []string{"run_shell_command"}},
	"web":   {Agent: []string{"web_fetch", "google_web_search"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }
