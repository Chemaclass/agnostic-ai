package claude

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// EmitAgents writes native agents for project or user-level sync.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, agent := range agents {
		body := emit.WithHeader(emit.DocumentStyled(agent.Meta, agent.MetaKeys, agent.MetaStyles, agent.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, agent.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
}
