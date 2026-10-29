package main

import (
	"fmt"
	"slices"
	"strings"
)

// builtinBaseline resolves --builtins-since to full commits. It is one
// revision for every target, or a "<target>=<rev>,..." list that
// scripts/target-facts.sh reads per target; an omitted target has none.
func (a app) builtinBaseline(value string, registered []string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.Contains(value, "=") {
		return a.command("git", "rev-parse", "--verify", value+"^{commit}")
	}
	var resolved []string
	seen := map[string]bool{}
	for _, pair := range strings.Split(value, ",") {
		target, rev, ok := strings.Cut(pair, "=")
		if !ok || rev == "" || !slices.Contains(registered, target) || seen[target] {
			return "", fmt.Errorf("invalid built-in baseline entry %q: want unique <target>=<rev>", pair)
		}
		seen[target] = true
		commit, err := a.command("git", "rev-parse", "--verify", rev+"^{commit}")
		if err != nil {
			return "", fmt.Errorf("built-in baseline for %s: %w", target, err)
		}
		resolved = append(resolved, target+"="+commit)
	}
	return strings.Join(resolved, ","), nil
}
