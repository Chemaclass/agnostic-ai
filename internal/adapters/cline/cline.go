package cline

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Cline (VSCode) reads `.clinerules/*.md` — one rule per file.
// No native agents, skills, or hooks.

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "cline" }

func (a *Adapter) Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := dirFor(cfg)

	for _, r := range spec.Filter(entries, spec.KindRule) {
		path := filepath.Join(dir, r.Name+".md")
		content := "# " + r.Name + "\n\n" + r.Body
		if err := emit.WriteFile(path, content, dryRun); err != nil {
			return err
		}
	}
	for _, ag := range spec.Filter(entries, spec.KindAgent) {
		path := filepath.Join(dir, "agent-"+ag.Name+".md")
		content := "# Agent: " + ag.Name + "\n\n" + ag.Body
		if err := emit.WriteFile(path, content, dryRun); err != nil {
			return err
		}
	}

	if len(spec.Filter(entries, spec.KindHook)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! cline: hooks not supported, skipped")
	}
	if len(spec.Filter(entries, spec.KindSkill)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! cline: skills not supported, skipped")
	}
	return nil
}

func dirFor(cfg *config.Config) string {
	if o, ok := cfg.Outputs["cline"]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return ".clinerules"
}
