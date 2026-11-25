package cli

import (
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func lintEnvironmentRequirements(envs []spec.Entry) []lintFinding {
	var findings []lintFinding
	for _, env := range envs {
		for source := &env; source != nil; source = source.Base {
			meta := source.Meta
			if source.Base != nil {
				meta = maps.Clone(source.LocalMeta)
				for _, field := range []string{spec.EnvironmentVariablesField, spec.EnvironmentAllowedDomainsField} {
					if meta[field] == nil {
						delete(meta, field)
					}
				}
			}
			_, issues := spec.ReadEnvironmentRequirements(meta)
			for _, key := range slices.Sorted(maps.Keys(meta)) {
				if !strings.HasPrefix(key, "x-") {
					continue
				}
				block, _ := meta[key].(map[string]any)
				overrides := make(map[string]any, 2)
				for _, field := range []string{spec.EnvironmentVariablesField, spec.EnvironmentAllowedDomainsField} {
					if v := block[field]; v != nil {
						overrides[field] = v
					}
				}
				_, overrideIssues := spec.ReadEnvironmentRequirements(overrides)
				for _, issue := range overrideIssues {
					issues = append(issues, "target override "+issue)
				}
			}
			for _, issue := range issues {
				findings = append(findings, lintFinding{Code: "LINT066", Severity: lintError, Path: source.Path, Message: issue})
			}
		}
	}
	return findings
}
