package emit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var claudeSyntaxOrder = []spec.ClaudeSyntax{spec.ClaudeShell, spec.ClaudeArguments, spec.ClaudePositional}

// claudeSyntaxExpanders lists, per spec kind, the targets whose own docs
// show them expanding a ClaudeSyntax in that kind's file. Every other
// target reads it as plain text.
var claudeSyntaxExpanders = map[spec.Kind]map[string][]spec.ClaudeSyntax{
	spec.KindSkill: {
		"claude": claudeSyntaxOrder,
	},
	spec.KindCommand: {
		"claude":   claudeSyntaxOrder,
		"opencode": claudeSyntaxOrder,
		"codex":    {spec.ClaudeArguments, spec.ClaudePositional},
		"factory":  {spec.ClaudeArguments},
		"augment":  {spec.ClaudeArguments},
		"kiro":     {spec.ClaudeArguments},
	},
}

// ExpandsClaudeSyntax reports whether target expands syntax in a spec of
// kind the way Claude Code does.
func ExpandsClaudeSyntax(target string, kind spec.Kind, syntax spec.ClaudeSyntax) bool {
	return slices.Contains(claudeSyntaxExpanders[kind][target], syntax)
}

// ReportClaudeBodySyntax raises one field no-op note per ClaudeSyntax
// that entries, already rendered for target, use and target leaves as
// plain text. mode is the project's on-unsupported policy: error fails
// on the first such line, silent reports nothing.
func ReportClaudeBodySyntax(target string, kind spec.Kind, entries []spec.Entry, mode string) error {
	if mode == OnUnsupportedSilent {
		return nil
	}
	for _, syntax := range claudeSyntaxOrder {
		if ExpandsClaudeSyntax(target, kind, syntax) {
			continue
		}
		var locations []string
		count := 0
		for _, e := range entries {
			lines := claudeSyntaxLines(e, target, syntax)
			if len(lines) == 0 {
				continue
			}
			count++
			locations = append(locations, lines...)
		}
		if count == 0 {
			continue
		}
		if mode == OnUnsupportedError {
			return fmt.Errorf("%s: %s reads %s in a %s as plain text; put the line in a ::target claude fence",
				locations[0], target, syntax, kind)
		}
		NoteFieldNoOp(target, kind, string(syntax), count,
			fmt.Sprintf("%s at %s; put the line in a ::target claude fence", claudeSyntaxEffect(syntax), strings.Join(locations, ", ")))
	}
	return nil
}

func claudeSyntaxLines(e spec.Entry, target string, syntax spec.ClaudeSyntax) []string {
	var out []string
	for _, use := range spec.FindClaudeSyntax(e.Body) {
		if use.Syntax == syntax && use.ReachesTarget(target) {
			out = append(out, e.BodyLocation(use.Line))
		}
	}
	return slices.Compact(out)
}

func claudeSyntaxEffect(syntax spec.ClaudeSyntax) string {
	if syntax == spec.ClaudeShell {
		return "the command does not run"
	}
	return "stays literal text"
}
