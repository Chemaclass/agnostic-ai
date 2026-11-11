package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintRulesModes reports an outputs.<target>.rules-mode value sync
// ignores (LINT061).
func lintRulesModes(cfg *config.Config, configPaths []string) []lintFinding {
	if cfg == nil {
		return nil
	}
	var findings []lintFinding
	for _, target := range slices.Sorted(maps.Keys(cfg.Outputs)) {
		mode := cfg.Outputs[target].RulesMode
		if mode == "" || config.RulesModes[target] == mode {
			continue
		}
		message := fmt.Sprintf("outputs.%s.rules-mode: %q has no effect: %s reads no rules-mode", target, mode, target)
		if want, ok := config.RulesModes[target]; ok {
			message = fmt.Sprintf("outputs.%s.rules-mode: %q has no effect: %s reads only %q", target, mode, target, want)
		}
		findings = append(findings, lintFinding{Code: "LINT061", Severity: lintWarn, Path: outputKeySource(configPaths, target, "rules-mode"), Message: message})
	}
	return findings
}
