package cli

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// filterTargets computes the effective target list from --only / --except.
// Exactly one of only or except should be non-empty; passing both is a caller
// error (the command layer enforces mutual exclusion before calling this).
// All names in only and except are validated against configured; unknown names
// return an error.
func filterTargets(configured, only, except []string) ([]string, error) {
	if len(only) > 0 {
		for _, name := range only {
			if !contains(configured, name) {
				return nil, notATargetError(name, configured)
			}
		}
		return only, nil
	}
	if len(except) > 0 {
		for _, name := range except {
			if !contains(configured, name) {
				return nil, notATargetError(name, configured)
			}
		}
		result := make([]string, 0, len(configured))
		for _, t := range configured {
			if !contains(except, t) {
				result = append(result, t)
			}
		}
		return result, nil
	}
	return configured, nil
}

// notATargetError explains why name is not among targets: a likely typo of
// one of them, a real adapter this run does not include, or no adapter.
func notATargetError(name string, targets []string) error {
	if s := adapters.SuggestName(name, targets); s != "" {
		return errs.Coded(errs.CodeSyncTargetUnknown, "unknown target: %s (did you mean %s?)", name, s)
	}
	if slices.Contains(adapters.Names(), name) {
		return errs.Coded(errs.CodeSyncTargetUnknown,
			"%s is not in this run's targets (%s); add it to agnostic-ai.yaml or pass -t %s",
			name, strings.Join(targets, ", "), name)
	}
	return errs.Coded(errs.CodeSyncTargetUnknown, "unknown target: %s", name)
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
