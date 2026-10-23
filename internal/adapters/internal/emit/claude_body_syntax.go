package emit

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// CommandSkillPreamble opens each command written as a skill. It asks
// for what the manual-only flag does where a tool drops the flag, carries
// the arguments a skill cannot fill in, and marks the file so import
// leaves it to the command spec.
const CommandSkillPreamble = "This skill is a command. Run it only when the user calls it by name, " +
	"and treat the text they type after the name as its arguments."

var claudeSyntaxOrder = []spec.ClaudeSyntax{spec.ClaudeShell, spec.ClaudeArguments, spec.ClaudePositional}

// claudeSyntaxExpanders lists, per spec kind, the targets whose own docs
// show them expanding a ClaudeSyntax in that kind's file at project scope.
var claudeSyntaxExpanders = map[spec.Kind]map[string][]spec.ClaudeSyntax{
	spec.KindSkill: {
		"codebuddy": claudeSyntaxOrder,
		"claude":    claudeSyntaxOrder,
	},
	spec.KindCommand: {
		"codebuddy": claudeSyntaxOrder,
		"claude":    claudeSyntaxOrder,
		"opencode":  claudeSyntaxOrder,
		"codex":     {spec.ClaudeArguments, spec.ClaudePositional},
		"factory":   {spec.ClaudeArguments},
		"augment":   {spec.ClaudeArguments},
		"kiro":      {spec.ClaudeArguments},
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
// on the first such line, silent reports nothing. Argument placeholders
// in a command written as a skill only get the note, since its preamble
// carries the arguments.
func ReportClaudeBodySyntax(target string, kind spec.Kind, entries []spec.Entry, mode string) error {
	return reportClaudeBodySyntax(target, kind, entries, mode, false)
}

// ReportGlobalSkillSyntax accounts for Kilo inline shell placeholders at trusted user paths.
func ReportGlobalSkillSyntax(target string, entries []spec.Entry, mode, home, dir string) error {
	trusted := false
	if target == "kilo" && filepath.IsAbs(home) && filepath.IsAbs(dir) {
		for _, root := range []string{".kilo", ".agents", ".claude"} {
			trusted = trusted || filepath.Clean(dir) == filepath.Join(home, root, "skills")
		}
	}
	return reportClaudeBodySyntax(target, spec.KindSkill, entries, mode, trusted)
}

func reportClaudeBodySyntax(target string, kind spec.Kind, entries []spec.Entry, mode string, inlineShell bool) error {
	if mode == OnUnsupportedSilent {
		return nil
	}
	for _, syntax := range claudeSyntaxOrder {
		if ExpandsClaudeSyntax(target, kind, syntax) {
			continue
		}
		var locations []string
		count, failAt := 0, ""
		for _, e := range entries {
			lines := claudeSyntaxLines(e, target, syntax, inlineShell)
			if len(lines) == 0 {
				continue
			}
			count++
			locations = append(locations, lines...)
			if failAt == "" && !argumentsCarried(e, kind, syntax) {
				failAt = lines[0]
			}
		}
		if count == 0 {
			continue
		}
		if mode == OnUnsupportedError && failAt != "" {
			return fmt.Errorf("%s: %s reads %s in a %s as plain text; put the line in a ::target claude fence",
				failAt, target, syntax, kind)
		}
		NoteFieldNoOp(target, kind, string(syntax), count,
			fmt.Sprintf("%s at %s; put the line in a ::target claude fence", claudeSyntaxEffect(syntax), strings.Join(locations, ", ")))
	}
	return nil
}

func argumentsCarried(e spec.Entry, kind spec.Kind, syntax spec.ClaudeSyntax) bool {
	return kind == spec.KindSkill && syntax != spec.ClaudeShell && strings.HasPrefix(e.Body, CommandSkillPreamble)
}

func claudeSyntaxLines(e spec.Entry, target string, syntax spec.ClaudeSyntax, inlineShell bool) []string {
	var out []string
	for _, use := range spec.FindClaudeSyntax(e.Body) {
		if use.Syntax == syntax && use.ReachesTarget(target) {
			if inlineShell && syntax == spec.ClaudeShell && !use.ShellBlock {
				continue
			}
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
