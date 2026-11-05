package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// secondsMCPTargets read an MCP timeout in seconds, so sync divides the
// spec's milliseconds by 1000.
var secondsMCPTargets = []string{"crush", "pi"}

// lintMCPTimeoutSeconds flags an MCP timeout under 1000 ms on a spec that
// reaches a seconds target. Import stored native seconds unchanged until
// #2997, so such a value is most likely seconds, and sync writes a limit
// of one second or less.
// Sync's rounding note shows only until the output is unchanged; this
// warning stays.
func lintMCPTimeoutSeconds(targets []string, b spec.Bundle) []lintFinding {
	reached := map[string][]string{}
	paths := map[string]string{}
	var names []string
	for _, target := range secondsMCPTargets {
		if !slices.Contains(targets, target) {
			continue
		}
		for _, m := range b.For(target).MCPs {
			if custom, ok := m.Meta["x-"+target].(map[string]any); ok && custom["timeout"] != nil {
				continue
			}
			ms, ok := m.Meta["timeout"].(int)
			if !ok || ms <= 0 || ms >= 1000 {
				continue
			}
			if _, seen := reached[m.Name]; !seen {
				names = append(names, m.Name)
				paths[m.Name] = m.Path
			}
			reached[m.Name] = append(reached[m.Name], target)
		}
	}
	var out []lintFinding
	for _, name := range names {
		verb := "reads"
		if len(reached[name]) > 1 {
			verb = "read"
		}
		out = append(out, lintFinding{Code: "LINT059", Severity: lintWarn, Path: paths[name], Message: fmt.Sprintf(
			"MCP server %q: timeout is milliseconds, and %s %s seconds, so sync writes a limit of one second or less. If the value is seconds, multiply it by 1000",
			name, strings.Join(reached[name], " and "), verb)})
	}
	return out
}
