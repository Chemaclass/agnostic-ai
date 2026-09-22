package claude

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// RenderAgent renders a target-filtered agent for project or user-level sync.
func RenderAgent(agent spec.Entry) string {
	return emit.WithHeader(emit.DocumentStyled(agent.Meta, agent.MetaKeys, agent.MetaStyles, agent.Body, target), emit.FormatMarkdown)
}
