package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintSharedRulesFiles reports a merged rules-file that sync replaces
// with another target's instructions file, which already carries its
// rules (LINT063). One that loses rules stops sync instead.
func lintSharedRulesFiles(cfg *config.Config, b spec.Bundle, targets, configPaths []string) []lintFinding {
	if cfg == nil {
		return nil
	}
	var findings []lintFinding
	for _, s := range projectSharedRulesFiles(cfg, b, targets) {
		if len(s.lost) > 0 {
			continue
		}
		findings = append(findings, lintFinding{Code: "LINT063", Severity: lintWarn, Path: outputKeySource(configPaths, s.owner, "rules-file"), Message: s.noEffect()})
	}
	return findings
}
