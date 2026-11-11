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

// Wrappers a tool puts around what the person typed: the opening one is
// dropped, and a closing one ends the line, so the link line before it
// stays whole.
var (
	typedOpen  = regexp.MustCompile(`<(user_query|command-name|command-args)\b[^>]*>`)
	typedClose = regexp.MustCompile(`</(user_query|command-name|command-args)>`)
)

// linkOf reads the link line off what the person typed: the stored
// prompt without the blocks a tool injects and the wrappers it adds,
// and without blank lines before it. The line itself is kept as typed,
// so spaces before it, two spaces or a tab stay text. When removal leaves
// an injected tag behind, a block was nested in another and the line
// may be text the tool added, so nothing links.
func linkOf(prompt string) (ref Ref, rest string, ok bool) {
	typed := injectedBlocks.ReplaceAllString(prompt, "")
	if injectedTag.MatchString(typed) {
		return Ref{}, prompt, false
	}
	typed = typedClose.ReplaceAllString(typedOpen.ReplaceAllString(typed, ""), "\n")
	return splitLink(strings.TrimLeft(typed, "\r\n"))
}

// splitLink reads the link line off the start of text and returns the
// rest. A carriage return may end the line. ok is false when the first
// line is not a link line; rest is then text as it was.
func splitLink(prompt string) (ref Ref, rest string, ok bool) {
	first, after, _ := strings.Cut(prompt, "\n")
	ref, ok = readLink(strings.TrimSuffix(first, "\r"))
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
