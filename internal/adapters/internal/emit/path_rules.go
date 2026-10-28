package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// pathRuleTargets maps each target that lazy-loads `paths:` rules from its
// own rules directory to the entry point that keeps its unscoped rules.
// A moved entry point can have other readers, so rules stay inlined there.
var pathRuleTargets = map[string]string{"qwen": "QWEN.md"}

// PathRulePatterns returns the file patterns target loads r for from its
// rules directory, or nil when r belongs in the entry point.
func PathRulePatterns(cfg *config.Config, target string, r spec.Entry) []string {
	entry, ok := pathRuleTargets[target]
	if !ok || HasLegacyRulesFile(cfg, target) || cfg != nil && EntryPointPath(cfg, target) != entry || r.EffectiveScope() != "" {
		return nil
	}
	m := ResolveMeta(r.Meta, target)
	if always, _ := m["alwaysApply"].(bool); always {
		return nil
	}
	raw, ok := m["paths"]
	if !ok {
		raw = m["globs"]
	}
	patterns := spec.GlobList(raw)
	if len(patterns) == 0 || spec.IsCatchAllGlobs(strings.Join(patterns, ",")) {
		return nil
	}
	return patterns
}
