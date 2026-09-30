package emit

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func codexGlobDirectories(cfg *config.Config, target string, r spec.Entry) ([]string, string) {
	if cfg == nil || target != "codex" && !slices.Contains(cfg.Targets, "codex") {
		return nil, ""
	}
	if opt := cfg.Outputs["codex"].NestedGlobRules; opt != nil && !*opt {
		return nil, ""
	}
	if target != "codex" && target != "cursor" &&
		(scopeDocument(target) != "AGENTS.md" || EntryPointPath(cfg, target) != EntryPointPath(cfg, "codex")) {
		return nil, ""
	}
	if scope, err := spec.RuleScope(r); err != nil || scope != "" {
		return nil, ""
	}
	if always, _ := r.Meta["alwaysApply"].(bool); always {
		return nil, ""
	}
	_, globs := r.Meta["globs"]
	_, paths := r.Meta["paths"]
	if !globs && !paths {
		return nil, ""
	}
	patterns, err := scopePatterns(r, "")
	if err != nil {
		return nil, err.Error()
	}
	if len(patterns) == 0 {
		return nil, ""
	}
	dirs, err := scopeDirectories(patterns)
	if err != nil {
		return nil, err.Error()
	}
	for _, reader := range append([]string{"codex", target}, cfg.Targets...) {
		if reader != "codex" && reader != "cursor" && scopeDocument(reader) != "AGENTS.md" && EntryPointPath(cfg, reader) != EntryPointPath(cfg, "codex") {
			continue
		}
		o := cfg.Outputs[reader]
		if o.File != "" || o.RulesFile != "" || o.RulesDir != "" || o.ProvenanceHeader != nil && !*o.ProvenanceHeader {
			return nil, fmt.Sprintf("%s output overrides do not support automatic nested instructions", reader)
		}
		if InlinesRulesIntoEntryPoint(reader) && scopeDocument(reader) != "AGENTS.md" {
			return nil, fmt.Sprintf("%s shares the root instructions but has no verified nested instructions", reader)
		}
	}
	return dirs, ""
}

func reportCodexGlobFallback(cfg *config.Config, target string, r spec.Entry, reason string) error {
	if target != "codex" || reason == "" {
		return nil
	}
	destination := OutputRulesFile(cfg, "codex", EntryPointPath(cfg, "codex"))
	message := fmt.Sprintf("%s: codex: rule %q is always loaded from %s (%s); use whole-subtree globs such as src/api/** or set outputs.codex.nested-glob-rules: false", r.Path, r.Name, destination, reason)
	switch cfg.OnUnsupported {
	case OnUnsupportedError:
		return fmt.Errorf("%s", message)
	case OnUnsupportedSilent:
		return nil
	default:
		NoteProject(message)
		return nil
	}
}
