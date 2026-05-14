// Package antigravity emits configs for Google Antigravity IDE.
//
// Antigravity reads project instructions from per-rule files under
// `.agent/rules/*.md`. This adapter emits those files.
//
// The `.agent/AGENTS.md` entry-point is written centrally by `sync`
// as a slim pointer to the source specs (one body shared with every
// other target's entry-point file). When `outputs.antigravity.rules-file`
// is set, this adapter instead writes the legacy merged layout at that
// path so users on older workflows keep their behavior.
//
// MCP support is omitted in this first cut: Antigravity's MCP surface
// was not confirmed in the public preview docs. Add it here once the
// upstream spec is stable.
package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target          = "antigravity"
	defaultRulesDir = ".agent/rules"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindRule},
}

// Adapter emits Antigravity configs.
type Adapter struct{}

// New returns an Antigravity adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes per-rule files under .agent/rules/ and, when opted in
// via outputs.antigravity.rules-file, a legacy merged document at
// that path. The `.agent/AGENTS.md` entry-point is written by `sync`,
// not here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultRulesDir),
		AgentPrefix: "agent-",
	}, dryRun); err != nil {
		return err
	}
	return emit.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun)
}
