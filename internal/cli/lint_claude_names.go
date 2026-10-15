package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintClaudeRefusedNames flags a skill or command whose name holds a
// character Claude Code refuses, so it never loads there (LINT047, warn).
func lintClaudeRefusedNames(b spec.Bundle, targets []string) []lintFinding {
	if !slices.Contains(targets, "claude") {
		return nil
	}
	var out []lintFinding
	for _, e := range append(append([]spec.Entry(nil), b.Skills...), b.Commands...) {
		if !e.EmitsTo("claude") {
			continue
		}
		chars := spec.ClaudeRefusedNameChars(e.Name)
		if len(chars) == 0 {
			continue
		}
		out = append(out, lintFinding{
			Code: "LINT047", Severity: lintWarn, Path: e.Path,
			Message: fmt.Sprintf("%s name %q holds %s, which Claude Code refuses, so the %s does not load there; rename it",
				e.Kind, e.Name, strings.Join(chars, ", "), e.Kind),
		})
	}
	return out
}
