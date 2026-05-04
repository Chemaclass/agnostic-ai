// Package windsurf emits .windsurf/rules/*.md for the Windsurf editor.
package windsurf

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target     = "windsurf"
	defaultDir = ".windsurf/rules"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule},
}

// Adapter emits Windsurf configs.
type Adapter struct{}

// New returns a Windsurf adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule and per agent into the rules directory.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	return emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         outDir(cfg),
		AgentPrefix: "agent-",
	}, dryRun)
}

func outDir(cfg *config.Config) string {
	if o, ok := cfg.Outputs[target]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return defaultDir
}
