// Package qoder emits .qoder/rules/*.md for Alibaba's Qoder IDE.
//
// Qoder reads project rules from `.qoder/rules/*.md`: one file per
// rule, each carrying a name and description, and Qoder's own docs
// state this native rules content takes precedence over `AGENTS.md`
// when both are present. Qoder's agent and skill surfaces have no
// documented file format yet, so this adapter declares only KindRule
// and leaves agents and skills to the unsupported-kind warning
// channel. Qoder also reads the cross-tool root `AGENTS.md`, which is
// written centrally by `sync` as a slim pointer to the source specs,
// not by this adapter.
package qoder

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target     = "qoder"
	defaultDir = ".qoder/rules"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule},
}

// Adapter emits Qoder configs.
type Adapter struct{}

// New returns a Qoder adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.qoder/rules`). Agents and skills have no native Qoder surface, so
// they are left to ReportUnsupported rather than flattened into the
// rules directory.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	return sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        emit.OutputRulesDir(cfg, target, defaultDir),
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun)
}
