package cli

import "fmt"

// filterTargets computes the effective target list from --only / --except.
// Exactly one of only or except should be non-empty; passing both is a caller
// error (the command layer enforces mutual exclusion before calling this).
// All names in only and except are validated against configured; unknown names
// return an error.
func filterTargets(configured, only, except []string) ([]string, error) {
	if len(only) > 0 {
		for _, name := range only {
			if !contains(configured, name) {
				return nil, fmt.Errorf("unknown target: %s", name)
			}
		}
		return only, nil
	}
	if len(except) > 0 {
		for _, name := range except {
			if !contains(configured, name) {
				return nil, fmt.Errorf("unknown target: %s", name)
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

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
