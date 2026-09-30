package cli

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func lintCodexPermissions(targets []string, cfg *config.Config, b spec.Bundle) ([]lintFinding, error) {
	if !slices.Contains(targets, "codex") {
		return nil, nil
	}
	mismatches, err := codex.PermissionPolicyDrift(b.For("codex").Settings, cfg)
	if err != nil {
		return nil, err
	}
	var findings []lintFinding
	for _, mismatch := range mismatches {
		effect := "no matching prefix"
		if mismatch.Decision != "" {
			effect = "a matching " + mismatch.Decision + " prefix"
		}
		findings = append(findings, lintFinding{Code: "LINT021", Severity: lintWarn, Path: mismatch.Path,
			Message: fmt.Sprintf("permissions.%s rule %s has %s in explicit Codex exec policies; align the permission and outputs.codex.exec-policies", mismatch.List, mismatch.Rule, effect)})
	}
	return findings, nil
}
