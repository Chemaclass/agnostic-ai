package conversations

import (
	"regexp"
	"strings"
)

// A conversation started from another one names it in a link line, the
// whole first line of its first prompt:
//
//	Continues <tool>:<id>
//
// Anything else is plain text and stays shown: another case, more
// words, an unknown tool, or an id not in that tool's form. A person
// may type the line by hand; it only decides where the row is drawn.
const linkWord = "Continues "

// linkIDs are the id forms each tool's store uses.
var linkIDs = map[Tool]*regexp.Regexp{
	ToolClaude: sessionIDPattern,
	ToolCodex:  sessionIDPattern,
	ToolCursor: cursorIDPattern,
}

// linkLine is the line that names ref as the conversation continued.
func linkLine(ref Ref) string {
	return linkWord + string(ref.Tool) + ":" + ref.ID
}

// readLink reads one whole line as a link line.
func readLink(line string) (Ref, bool) {
	rest, ok := strings.CutPrefix(line, linkWord)
	if !ok {
		return Ref{}, false
	}
	tool, id, ok := strings.Cut(rest, ":")
	pattern, known := linkIDs[Tool(tool)]
	if !ok || !known || !pattern.MatchString(id) {
		return Ref{}, false
	}
	return Ref{Tool: Tool(tool), ID: id}, true
}

// splitLink reads the link line off the start of a prompt as the store
// keeps it, so a line with spaces before it, a tab, or a wrapper stays
// text. It returns the rest. ok is false when the first line is not a
// link line; rest is then the prompt as it was.
func splitLink(prompt string) (ref Ref, rest string, ok bool) {
	first, after, _ := strings.Cut(prompt, "\n")
	ref, ok = readLink(first)
	if !ok {
		return Ref{}, prompt, false
	}
	return ref, strings.TrimSpace(after), true
}

// cutLink drops ref's link line from the start of a title that starts
// with that whole line: Codex keeps the first prompt as the thread's
// title.
func cutLink(text string, ref Ref) string {
	line := linkLine(ref)
	if text == line {
		return ""
	}
	for _, gap := range []string{"\n", " "} {
		if rest, ok := strings.CutPrefix(text, line+gap); ok {
			return strings.TrimSpace(rest)
		}
	}
	return text
}
