package continueai

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Continue (continue.dev) reads `.continue/rules/*.md` for project rules.
// Agents render as additional rule files.

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "continue" }

func (a *Adapter) Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := dirFor(cfg)

	for _, r := range spec.Filter(entries, spec.KindRule) {
		path := filepath.Join(dir, r.Name+".md")
		if err := emit.WriteFile(path, "# "+r.Name+"\n\n"+r.Body, dryRun); err != nil {
			return err
		}
	}
	for _, ag := range spec.Filter(entries, spec.KindAgent) {
		path := filepath.Join(dir, "agent-"+ag.Name+".md")
		if err := emit.WriteFile(path, "# Agent: "+ag.Name+"\n\n"+ag.Body, dryRun); err != nil {
			return err
		}
	}

	if len(spec.Filter(entries, spec.KindHook)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! continue: hooks not supported, skipped")
	}
	if len(spec.Filter(entries, spec.KindSkill)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! continue: skills not supported, skipped")
	}
	return nil
}

func dirFor(cfg *config.Config) string {
	if o, ok := cfg.Outputs["continue"]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return ".continue/rules"
}
