package cursor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "cursor" }

func (a *Adapter) Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := rulesDir(cfg)

	for _, r := range spec.Filter(entries, spec.KindRule) {
		path := filepath.Join(dir, r.Name+".mdc")
		if err := emit.WriteFile(path, mdc(r, true), dryRun); err != nil {
			return err
		}
	}
	for _, a := range spec.Filter(entries, spec.KindAgent) {
		path := filepath.Join(dir, a.Name+".mdc")
		if err := emit.WriteFile(path, mdc(a, false), dryRun); err != nil {
			return err
		}
	}

	if len(spec.Filter(entries, spec.KindHook)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! cursor: hooks not supported, skipped")
	}
	if len(spec.Filter(entries, spec.KindSkill)) > 0 {
		fmt.Fprintln(os.Stderr, "  ! cursor: skills not supported, skipped")
	}
	return nil
}

func mdc(e spec.Entry, alwaysApplyDefault bool) string {
	desc, _ := e.Meta["description"].(string)
	globs, _ := e.Meta["globs"].(string)
	if globs == "" {
		globs = "**/*"
	}
	always := alwaysApplyDefault
	if v, ok := e.Meta["alwaysApply"].(bool); ok {
		always = v
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: " + desc + "\n")
	b.WriteString("globs: " + globs + "\n")
	b.WriteString(fmt.Sprintf("alwaysApply: %t\n", always))
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func rulesDir(cfg *config.Config) string {
	if o, ok := cfg.Outputs["cursor"]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return ".cursor/rules"
}
