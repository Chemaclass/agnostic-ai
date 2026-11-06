package junie

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// maxMCPTimeoutMs is the longest per-server MCP timeout Junie accepts.
// junie.jetbrains.com/docs/junie-cli-mcp-configuration.html: "The value
// must be between 1000 (1 second) and 3600000 (1 hour). Values outside
// this range are ignored, a warning is logged, and the 60-second default
// is used."
const maxMCPTimeoutMs = 3600000

// noteMCPTimeoutsAboveLimit writes each value as given, since Junie logs its
// own warning, and tells the user Junie will use 60 seconds instead.
func noteMCPTimeoutsAboveLimit(mcps []spec.Entry) {
	for _, m := range mcps {
		ms, ok := emit.IntField(emit.ResolveMeta(m.Meta, target), "timeout")
		if !ok || ms <= maxMCPTimeoutMs {
			continue
		}
		emit.NoteProject(fmt.Sprintf("%s: MCP %s timeout %d ms is above Junie's limit of %d ms, so Junie ignores it and uses 60 seconds", target, m.Name, ms, maxMCPTimeoutMs))
	}
}
