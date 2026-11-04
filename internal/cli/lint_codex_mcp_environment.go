package cli

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintCodexMCPEnvironment flags the old Codex-only `experimental_environment`
// field. Codex reads `environment_id` instead (#2976), so sync writes
// `local` as `environment_id` and leaves any other value out.
func lintCodexMCPEnvironment(targets []string, b spec.Bundle) []lintFinding {
	if !slices.Contains(targets, "codex") {
		return nil
	}
	var out []lintFinding
	for _, m := range b.For("codex").MCPs {
		old, ok := m.Meta["experimental_environment"]
		if !ok {
			continue
		}
		msg := fmt.Sprintf("MCP server %q: experimental_environment: %v names no executor, so sync leaves it out for Codex; set environment_id to a configured executor ID, or remove the key", m.Name, old)
		switch {
		case m.Meta["environment_id"] != nil:
			msg = fmt.Sprintf("MCP server %q: environment_id replaces experimental_environment, which sync ignores for Codex; remove experimental_environment", m.Name)
		case old == "local":
			msg = fmt.Sprintf("MCP server %q: write environment_id: local in place of experimental_environment: local. `agnostic-ai migrate --only mcps` does it", m.Name)
		}
		out = append(out, lintFinding{Code: "LINT058", Severity: lintWarn, Path: m.Path, Message: msg})
	}
	return out
}
