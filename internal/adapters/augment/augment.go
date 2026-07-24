// Package augment emits configs for Augment Code.
//
// Augment reads project instructions from the root `AGENTS.md` file,
// written centrally by `sync` as a slim pointer to the source specs
// with rule bodies inlined by default (one body shared with every
// other AGENTS.md-family target). Augment also reads a second, older
// root file, `.augment-guidelines`: a single concatenated document of
// project guidelines. This adapter writes that document only when the
// user opts in via `outputs.augment.rules-file`, so a fresh `init`
// followed by `sync` never surprises a project with an extra root
// file.
//
// `.augment-guidelines` reuses the same generic legacy-rules-file
// mechanism the zed, aider, warp, and antigravity adapters use for
// their own opt-in merged documents (`Session.EmitLegacyRulesFile`);
// there is no dedicated `guidelines-file` config key. Only rule bodies
// go into the document: Augment has no per-agent or per-skill file
// surface, so caps.Supports declares KindRule only.
package augment

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "augment"

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule},
}

// Adapter emits Augment configs.
type Adapter struct{}

// New returns an Augment adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the legacy concatenated `.augment-guidelines`-style
// document only when `outputs.augment.rules-file` is set, scoped to
// rules so an agent or skill spec targeted at augment (which has no
// native surface for either) never leaks into the document. The root
// AGENTS.md entry-point (with rule bodies inlined) is written
// centrally by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	return sess.EmitLegacyRulesFile(spec.Bundle{Rules: b.Rules}, cfg, target, emit.MergedOpts{
		Title: "Project guidelines",
	}, dryRun)
}
