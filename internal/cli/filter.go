package cli

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
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

// validateConfigTargets fails on a config target that is a likely typo of
// a built-in one, before sync removes the real target's files. A name with
// no close match may be an external adapter missing from this PATH, which
// sync reports as a warning.
func validateConfigTargets(cfg *config.Config, source string) error {
	for _, t := range cfg.Targets {
		if slices.Contains(adapters.Names(), t) {
			continue
		}
		s := suggest.Name(t, adapters.Names())
		if s == "" {
			continue
		}
		if _, err := adapters.Resolve(t); err != nil {
			return errs.Coded(errs.CodeSyncTargetUnknown, "%s: targets: unknown target %q (did you mean %s?)", source, t, s)
		}
	}
	return nil
}

// notATargetError explains why name is not among targets: a likely typo of
// one of them, a real adapter this run does not include, or no adapter.
// Callers differ in where their targets come from (config, -t, the global
// set), so the message states the fact and leaves the remedy out.
func notATargetError(name string, targets []string) error {
	if s := suggest.Name(name, targets); s != "" {
		return errs.Coded(errs.CodeSyncTargetUnknown, "unknown target: %s (did you mean %s?)", name, s)
	}
	if slices.Contains(adapters.Names(), name) {
		if len(targets) == 0 {
			return errs.Coded(errs.CodeSyncTargetUnknown, "%s is not in this run's targets", name)
		}
		return errs.Coded(errs.CodeSyncTargetUnknown, "%s is not in this run's targets (%s)", name, strings.Join(targets, ", "))
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
