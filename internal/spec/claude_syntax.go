package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// ClaudeSyntax is body syntax Claude Code expands in a skill or command
// before the model reads it. Other tools read it as plain text.
type ClaudeSyntax string

const (
	// ClaudeShell is dynamic context: a !`command` line or a ```! block
	// whose output Claude Code inlines.
	ClaudeShell ClaudeSyntax = "!`command`"
	// ClaudeArguments is the $ARGUMENTS placeholder.
	ClaudeArguments ClaudeSyntax = "$ARGUMENTS"
	// ClaudePositional is an indexed placeholder: $0, $1, or $ARGUMENTS[N].
	ClaudePositional ClaudeSyntax = "$N"
)

// ClaudeSyntaxUse is one body line that uses a ClaudeSyntax.
type ClaudeSyntaxUse struct {
	Syntax ClaudeSyntax
	// Line is 1-based within the body.
	Line int
	// Fence is the allow-list of the ::target fence around the line, nil
	// outside a fence.
	Fence []string
	// Fenced is true inside a ::target fence, even a bare one that allows
	// no target.
	Fenced bool
}

// ReachesTarget reports whether target reads the line after ::target
// fences resolve.
func (u ClaudeSyntaxUse) ReachesTarget(target string) bool {
	return !u.Fenced || inAllowList(u.Fence, target)
}

// Claude Code runs the inline form only when `!` opens the line or
// follows whitespace.
var (
	claudeShellRe      = regexp.MustCompile("(?:^|\\s)!`[^`]+`")
	claudeArgumentsRe  = regexp.MustCompile(`(^|[^\\])\$ARGUMENTS(\[\d+\])?`)
	claudePositionalRe = regexp.MustCompile(`(^|[^\\])\$\d`)
)

// FindClaudeSyntax returns each line of body that uses ClaudeSyntax,
// one use per line and shape, in line order. Fenced code blocks hold
// examples, so only $ARGUMENTS counts inside one: a shell `$1` or a
// sample !`command` there is not meant for Claude Code. A ```! opener
// is itself a ClaudeShell use. A backslash before `$` escapes it, as in
// Claude Code.
func FindClaudeSyntax(body string) []ClaudeSyntaxUse {
	// Every syntax needs a `$` or a `!`; most bodies have neither, and the
	// regexps cost most of a sync that changes nothing.
	if !strings.ContainsAny(body, "$!") {
		return nil
	}
	var out []ClaudeSyntaxUse
	var fence []string
	fenced := false
	code := ""
	for i, line := range strings.Split(body, "\n") {
		add := func(s ClaudeSyntax) {
			out = append(out, ClaudeSyntaxUse{Syntax: s, Line: i + 1, Fence: fence, Fenced: fenced})
		}
		switch marker, allow := parseFenceMarker(line); marker {
		case fenceTargetOpen:
			fence, fenced = allow, true
			continue
		case fenceTargetClose:
			fence, fenced = nil, false
			continue
		}
		if marker := fenceMarker(line); marker != "" {
			switch {
			case code == "":
				code = marker
				if strings.TrimSpace(strings.TrimLeft(line, " ")[len(marker):]) == "!" {
					add(ClaudeShell)
				}
			case strings.HasPrefix(marker, code):
				code = ""
			}
			continue
		}
		if !strings.ContainsAny(line, "$!") {
			continue
		}
		if code == "" && claudeShellRe.MatchString(line) {
			add(ClaudeShell)
		}
		plain, indexed := claudeArgumentsUses(line)
		if plain {
			add(ClaudeArguments)
		}
		if indexed || code == "" && claudePositionalRe.MatchString(line) {
			add(ClaudePositional)
		}
	}
	return out
}

// claudeArgumentsUses reports whether line holds a bare $ARGUMENTS and
// whether it holds an indexed $ARGUMENTS[N].
func claudeArgumentsUses(line string) (plain, indexed bool) {
	for _, m := range claudeArgumentsRe.FindAllStringSubmatch(line, -1) {
		if m[2] == "" {
			plain = true
		} else {
			indexed = true
		}
	}
	return plain, indexed
}

// BodyLocation names body line line (1-based) for a message: the file
// and line when BodyLine maps the body onto Path, else the path alone,
// else the spec name.
func (e Entry) BodyLocation(line int) string {
	switch {
	case e.Path == "":
		return e.Name
	case e.BodyLine > 0:
		return fmt.Sprintf("%s:%d", e.Path, e.BodyLine+line-1)
	default:
		return e.Path
	}
}
