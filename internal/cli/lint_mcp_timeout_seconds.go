package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// subSecondMCPTargets get a limit of one second or less from a timeout
// under 1000 ms: Crush and Pi read seconds, and Junie reads milliseconds
// from 1000 up.
var subSecondMCPTargets = []string{"crush", "pi", "junie"}

// junieMaxMCPTimeoutMs is the longest MCP timeout Junie accepts; above it
// Junie uses 60 seconds.
const junieMaxMCPTimeoutMs = 3600000

// lintMCPTimeoutSeconds flags an MCP timeout that a target will not use
// as written: under 1000 ms on a spec that reaches one of
// subSecondMCPTargets, or above one hour on a spec that reaches Junie.
// Import stored Crush and Pi seconds unchanged until #2997, so a value
// under 1000 is most likely seconds. Sync's notes show only until the
// output is unchanged; this warning stays.
func lintMCPTimeoutSeconds(targets []string, b spec.Bundle) []lintFinding {
	reached := map[string][]string{}
	paths := map[string]string{}
	var names []string
	for _, target := range subSecondMCPTargets {
		if !slices.Contains(targets, target) {
			continue
		}
		for _, m := range b.For(target).MCPs {
			if custom, ok := m.Meta["x-"+target].(map[string]any); ok && custom["timeout"] != nil {
				continue
			}
			ms, ok := adapters.WholeNumber(m.Meta["timeout"])
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
		tools := reached[name]
		named, verb := tools[0], "gets"
		if n := len(tools); n > 1 {
			named, verb = strings.Join(tools[:n-1], ", ")+" and "+tools[n-1], "get"
		}
		out = append(out, lintFinding{Code: "LINT059", Severity: lintWarn, Path: paths[name], Message: fmt.Sprintf(
			"MCP server %q: timeout is milliseconds, so %s %s a limit of one second or less. If the value is seconds, multiply it by 1000",
			name, named, verb)})
	}
	return append(out, lintJunieMCPTimeoutAboveLimit(targets, b)...)
}

func lintJunieMCPTimeoutAboveLimit(targets []string, b spec.Bundle) []lintFinding {
	if !slices.Contains(targets, "junie") {
		return nil
	}
	var out []lintFinding
	for _, m := range b.For("junie").MCPs {
		field, raw := "timeout", m.Meta["timeout"]
		if custom, ok := m.Meta["x-junie"].(map[string]any); ok && custom["timeout"] != nil {
			field, raw = "x-junie.timeout", custom["timeout"]
		}
		ms, ok := adapters.WholeNumber(raw)
		if !ok || ms <= junieMaxMCPTimeoutMs {
			continue
		}
		out = append(out, lintFinding{Code: "LINT059", Severity: lintWarn, Path: m.Path, Message: fmt.Sprintf(
			"MCP server %q: %s %d is above Junie's limit of %d ms (one hour), so Junie ignores it and uses 60 seconds. Lower it to %d or less",
			m.Name, field, ms, junieMaxMCPTimeoutMs, junieMaxMCPTimeoutMs)})
	}
	return out
}
