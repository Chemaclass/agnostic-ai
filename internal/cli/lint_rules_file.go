package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintRulesFiles reports an outputs.<target>.rules-file on a built-in
// target whose adapter writes no rules file, which sync ignores
// (LINT062).
func lintRulesFiles(cfg *config.Config, configPaths []string) []lintFinding {
	if cfg == nil {
		return nil
	}
	var findings []lintFinding
	for _, target := range slices.Sorted(maps.Keys(cfg.Outputs)) {
		if cfg.Outputs[target].RulesFile == "" || !adapters.IgnoresRulesFile(target) {
			continue
		}
		message := fmt.Sprintf("outputs.%s.rules-file has no effect: %s writes no rules file, so sync ignores the key", target, target)
		findings = append(findings, lintFinding{Code: "LINT062", Severity: lintWarn, Path: outputKeySource(configPaths, target, "rules-file"), Message: message})
	}
	return findings
}
