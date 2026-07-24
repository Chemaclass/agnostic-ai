// Package jules emits configs for Google Jules, the cloud-hosted coding
// agent.
//
// Jules reads project instructions exclusively from the root
// `AGENTS.md` file. It has no native per-project config directory and
// no documented per-rule, per-agent, or per-skill file surface, so
// there is nothing for this adapter to write: the entry-point (with
// rule bodies inlined) is rendered centrally by `sync`, the same file
// every other AGENTS.md-family target shares.
//
// KindRule is still declared in caps.Supports even though Emit never
// writes a file itself: rules reach Jules exclusively through that
// shared AGENTS.md entry-point, so declaring it keeps the
// "unsupported" warning honest (rules genuinely do reach Jules). See
// the crush and warp adapters for the same convention. Agents and
// skills have no route to Jules at all, so they are absent from
// Supports and surface through the standard unsupported-kind warning.
package jules

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "jules"

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule},
}

// Adapter emits Jules configs. There is nothing to emit: see the
// package doc.
type Adapter struct{}

// New returns a Jules adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit reports any spec kind Jules cannot carry and writes nothing:
// rules reach Jules through the shared AGENTS.md entry-point that
// `sync` writes centrally, and Jules has no other native file surface
// for agents or skills to fall back to.
func (Adapter) Emit(_ *emit.Session, b spec.Bundle, cfg *config.Config, _ bool) error {
	return emit.ReportUnsupported(caps, b, cfg.OnUnsupported)
}
