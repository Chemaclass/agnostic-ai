package cli

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func lintCopilotSetupSteps(targets []string, b spec.Bundle) []lintFinding {
	if !slices.Contains(targets, "copilot") {
		return nil
	}
	var findings []lintFinding
	for _, e := range copilot.SharedSetupStepsSources(b.Environments) {
		if !e.EmitsTo("copilot") {
			continue
		}
		for _, issue := range copilot.SetupStepsIssues(e) {
			findings = append(findings, lintFinding{Code: "LINT065", Severity: lintError, Path: e.Path, Message: issue})
		}
	}
	return findings
}
