// Package trae emits .trae/rules/*.md for Trae, ByteDance's AI IDE.
//
// Trae reads project rules from `.trae/rules/*.md` as persistent
// behavioral constraints; it also applies `.trae/rules/` folders found
// in subdirectories, but this adapter targets the project root only.
// `project_rules.md` is the older single-file location and is not
// emitted here. Trae's custom-agent and skill surfaces have no
// documented file format yet, so agents and skills flatten to rule-form
// files alongside plain rules (the same approach cline uses). Trae also
// reads the cross-tool root `AGENTS.md`, which is written centrally by
// `sync`, not by this adapter.
package trae

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target     = "trae"
	defaultDir = ".trae/rules"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule},
}

// Adapter emits Trae configs.
type Adapter struct{}

// New returns a Trae adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule, agent, and skill into the rules
// directory (default `.trae/rules`).
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	return sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
	}, dryRun)
}
