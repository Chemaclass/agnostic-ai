// Package goose emits configs for Block's Goose CLI.
//
// Goose reads project instructions from the root `AGENTS.md` file,
// written centrally by `sync` as a slim pointer to the source specs
// with rule bodies inlined by default (one body shared with every
// other AGENTS.md-family target). Goose also reads a second, older
// file, `.goosehints`: a single concatenated document of project
// hints. This adapter writes that document only when the user opts in
// via `outputs.goose.rules-file`, so a fresh `init` followed by `sync`
// never surprises a project with an extra root file.
//
// `.goosehints` reuses the same generic legacy-rules-file mechanism the
// zed, aider, warp, and antigravity adapters use for their own opt-in
// merged documents (`Session.EmitLegacyRulesFile`); there is no
// dedicated `hints-file` config key. Only rule bodies go into the
// document: Goose has no per-agent or per-skill file surface, so
// caps.Supports declares KindRule only.
package goose

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "goose"

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule},
}

// Adapter emits Goose configs.
type Adapter struct{}

// New returns a Goose adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the legacy concatenated `.goosehints`-style document only
// when `outputs.goose.rules-file` is set, scoped to rules so an agent
// or skill spec targeted at goose (which has no native surface for
// either) never leaks into the document. The root AGENTS.md
// entry-point (with rule bodies inlined) is written centrally by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	return sess.EmitLegacyRulesFile(spec.Bundle{Rules: b.Rules}, cfg, target, emit.MergedOpts{
		Title: "Project rules",
	}, dryRun)
}
