package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintClaudeBodySyntax flags each skill or command line that uses Claude
// Code body syntax an enabled target reads as plain text (LINT019, warn).
func lintClaudeBodySyntax(b spec.Bundle, targets []string, support kindSupport) []lintFinding {
	var out []lintFinding
	for _, e := range append(append([]spec.Entry(nil), b.Skills...), b.Commands...) {
		for _, use := range spec.FindClaudeSyntax(e.Body) {
			readers := plainTextReaders(e, use, targets, support)
			if len(readers) == 0 {
				continue
			}
			out = append(out, lintFinding{
				Code: "LINT019", Severity: lintWarn, Path: e.Path,
				Message: fmt.Sprintf("%s: %s %s %s as plain text, so %s; put the line in a ::target claude fence",
					bodyLineLabel(e, use.Line), strings.Join(readers, ", "), readVerb(len(readers)), use.Syntax, claudeSyntaxLoss(use.Syntax)),
			})
		}
	}
	return out
}

func plainTextReaders(e spec.Entry, use spec.ClaudeSyntaxUse, targets []string, support kindSupport) []string {
	var out []string
	for _, t := range targets {
		if _, ok := support[e.Kind][t]; !ok || !e.EmitsTo(t) || !use.ReachesTarget(t) {
			continue
		}
		if !adapters.ExpandsClaudeSyntax(t, e.Kind, use.Syntax) {
			out = append(out, t)
		}
	}
	return out
}

func bodyLineLabel(e spec.Entry, line int) string {
	if e.BodyLine > 0 {
		return fmt.Sprintf("line %d", e.BodyLine+line-1)
	}
	return fmt.Sprintf("body line %d", line)
}

func readVerb(n int) string {
	if n == 1 {
		return "reads"
	}
	return "read"
}

func claudeSyntaxLoss(syntax spec.ClaudeSyntax) string {
	if syntax == spec.ClaudeShell {
		return "the command does not run"
	}
	return "it stays literal text"
}
