package adapters

import (
	"fmt"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgentEmitter writes only native agent definitions to the supplied directory.
// Project migrations and other configuration writes stay in Adapter.Emit.
type AgentEmitter interface {
	EmitAgents(sess *Session, agents []spec.Entry, dir string, dryRun bool) error
}

// RenderAgents plans native agent files without touching project configuration
// or disk. It applies the same target selection and body fences as project sync.
func RenderAgents(target string, agents []spec.Entry, dir string) ([]CapturedFile, error) {
	b := (spec.Bundle{Agents: agents}).For(target)
	// Nothing to place: a misconfigured root must not fail unrelated surfaces.
	if len(b.Agents) == 0 {
		return nil, nil
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("%s agents: native directory %q must be absolute; check the tool's configuration root environment variable", target, dir)
	}
	a, err := Resolve(target)
	if err != nil {
		return nil, err
	}
	emitter, ok := a.(AgentEmitter)
	if !ok {
		return nil, fmt.Errorf("%s: native agent emission is unsupported", target)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := emitter.EmitAgents(sess, b.Agents, dir, false); err != nil {
		return nil, fmt.Errorf("%s agents: %w", target, err)
	}
	return sess.StopCapture(), nil
}
