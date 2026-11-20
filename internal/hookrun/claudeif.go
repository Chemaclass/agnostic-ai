package hookrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// claudeIfEvents are the events Claude Code evaluates `if` on. "On
// other events, a hook with `if` set never runs" (code.claude.com/docs/en/hooks).
var claudeIfEvents = []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionDenied"}

// claudeEditTools are the tools an `Edit(...)` rule covers: "Edit rules
// apply to all built-in tools that edit files" (code.claude.com/docs/en/permissions).
var claudeEditTools = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}

// ClaudeIfRuns reports whether Claude Code runs a handler whose `if` is
// rule for the tool call in body. The rule is one permission rule,
// `Tool` or `Tool(specifier)`, matched against the tool name and input.
// A Bash rule runs the hook when any subcommand matches, and whenever
// the command cannot be split, as Claude Code does.
func ClaudeIfRuns(rule, event string, body []byte, root string) (bool, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true, nil
	}
	if !slices.Contains(claudeIfEvents, event) {
		return false, nil
	}
	var call struct {
		ToolName  string         `json:"tool_name"`
		ToolInput map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		return false, fmt.Errorf("read the payload for if: %w", err)
	}
	tool, spec, hasSpec := strings.Cut(rule, "(")
	if hasSpec {
		var ok bool
		if spec, ok = strings.CutSuffix(spec, ")"); !ok {
			return false, fmt.Errorf("if %q: missing closing parenthesis", rule)
		}
	}
	isEdit := tool == "Edit" && slices.Contains(claudeEditTools, call.ToolName)
	if !isEdit && !globMatch(tool, call.ToolName) {
		return false, nil
	}
	if !hasSpec || spec == "*" {
		return true, nil
	}
	if name, value, ok := parameterRule(spec, call.ToolInput); ok {
		got, set := call.ToolInput[name]
		return set && globMatch(value, fmt.Sprint(got)), nil
	}
	switch {
	case call.ToolName == "Bash":
		command, _ := call.ToolInput["command"].(string)
		return bashRuleRuns(spec, command), nil
	case tool == "Edit" || tool == "Read":
		file, _ := call.ToolInput["file_path"].(string)
		if file == "" {
			file, _ = call.ToolInput["notebook_path"].(string)
		}
		return file != "" && pathRuleMatches(spec, file, root), nil
	}
	return false, nil
}

var paramName = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$`)

// parameterRule reads `param:value`, which matches a top-level input
// field. `ls:*` on Bash is the legacy prefix form instead, so a name the
// input does not carry is a parameter only when its value is no `*`.
func parameterRule(spec string, input map[string]any) (name, value string, ok bool) {
	m := paramName.FindStringSubmatch(spec)
	if m == nil {
		return "", "", false
	}
	if _, set := input[m[1]]; set || strings.TrimSpace(m[2]) != "*" {
		return m[1], strings.TrimSpace(m[2]), true
	}
	return "", "", false
}

func globMatch(pattern, value string) bool { return matchGlob(pattern, value, "") }

// bashGlobMatch lets `*` cross the newlines a quoted argument can hold.
func bashGlobMatch(pattern, value string) bool { return matchGlob(pattern, value, "(?s)") }

func matchGlob(pattern, value, flags string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(flags + "^" + strings.Join(parts, ".*") + "$").MatchString(value)
}

// bashRuleRuns follows the Bash rows of the hooks reference: leading
// `VAR=value` assignments are stripped, each subcommand of `&&`, `||`,
// `;`, `|`, `|&`, `&`, and newlines is checked, so are commands inside
// `$()` and backticks, and a pattern that names more than the command
// runs on any `$()`, backtick, or `$VAR`.
func bashRuleRuns(pattern, command string) bool {
	if p, ok := strings.CutSuffix(pattern, ":*"); ok {
		pattern = p + " *"
	}
	subcommands, ok := splitBash(command)
	if !ok {
		return true
	}
	literal, _, _ := strings.Cut(pattern, "*")
	if len(strings.Fields(literal)) > 1 && strings.ContainsAny(command, "`$") {
		return true
	}
	for _, sub := range subcommands {
		command := stripBashPrefix(sub)
		if commandUnknown(command) {
			return true
		}
		if bashPatternMatches(pattern, command) {
			return true
		}
	}
	return false
}

// dequoteCommandName removes the quotes and backslashes in the first
// word, as bash does before it looks the command up: `\git` and `"g"it`
// run git.
func dequoteCommandName(command string) string {
	end := strings.IndexFunc(command, unicode.IsSpace)
	if end < 0 {
		end = len(command)
	}
	word := []rune(command[:end])
	if !strings.ContainsAny(string(word), `\'"`) {
		return command
	}
	var b strings.Builder
	for i := 0; i < len(word); i++ {
		switch {
		case word[i] == '\\' && i+1 < len(word):
			i++
			b.WriteRune(word[i])
		case word[i] == '\'' || word[i] == '"':
		default:
			b.WriteRune(word[i])
		}
	}
	return b.String() + command[end:]
}

var assignmentWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)

// commandUnknown reports whether the program command runs cannot be read
// from its text: eval runs its arguments as a new command line, and a
// substitution or variable in the name decides it at run time.
func commandUnknown(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || assignmentWord.MatchString(fields[0]) {
		return false
	}
	return fields[0] == "eval" || strings.ContainsAny(fields[0], substitutionOutput+"$")
}

func bashPatternMatches(pattern, command string) bool {
	if bashGlobMatch(pattern, command) {
		return true
	}
	// "A `*` at the end, with a space before it, also matches the bare
	// command", when it is the only wildcard.
	bare, ok := strings.CutSuffix(pattern, " *")
	return ok && !strings.Contains(bare, "*") && bare == command
}

// bashWrappers are stripped before matching (permissions "Wrappers").
var bashWrappers = []string{"time", "nice", "nohup", "builtin", "noglob", "command"}

var leadingAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=(?:'[^']*'|"[^"]*"|\S*)\s+`)

// leadingRedirection is a redirection and its word before the command
// name, such as `2>&1` or `<<<word`.
var leadingRedirection = regexp.MustCompile(`(?s)^[0-9]*(?:<<<|<<-?|&>>|&>|>>|>&|<&|<>|>\||<|>)\s*(?:'[^']*'|"(?:\\.|[^"\\])*"|\\.|[^\s'"\\;&|<>()])+`)

func stripBashPrefix(command string) string {
	for {
		command = strings.TrimSpace(command)
		if loc := leadingRedirection.FindStringIndex(command); loc != nil {
			command = command[loc[1]:]
			continue
		}
		if loc := leadingAssignment.FindStringIndex(command); loc != nil {
			command = command[loc[1]:]
			continue
		}
		command = dequoteCommandName(command)
		fields := strings.Fields(command)
		switch {
		case len(fields) > 1 && fields[0] == "command" && fields[1] == "-v":
			return command
		case len(fields) > 1 && slices.Contains(bashWrappers, fields[0]):
			command = strings.TrimPrefix(command, fields[0])
		case len(fields) > 2 && fields[0] == "timeout":
			command = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(command, fields[0])), fields[1])
		case len(fields) > 1 && fields[0] == "xargs" && !strings.HasPrefix(fields[1], "-"):
			command = strings.TrimPrefix(command, fields[0])
		default:
			return command
		}
	}
}

// splitBash returns the subcommands of command, with the commands inside
// `$()` and backticks as subcommands of their own. ok is false when the
// command does not parse: an open quote, substitution, or operator.
//
// It reads command twice and keeps the subcommands of both readings:
// once with comments and here-documents as plain shell text, and once
// the way bash reads them, where they are not commands but the
// substitutions in an unquoted here-document body are. A misread
// comment or here-document can then only add a subcommand, never hide
// one.
func splitBash(command string) ([]string, bool) {
	plain, ok := splitBashAs(command, false)
	if !ok {
		return nil, false
	}
	aware, ok := splitBashAs(command, true)
	if !ok {
		return nil, false
	}
	return append(plain, aware...), true
}

func splitBashAs(command string, aware bool) (subs []string, ok bool) {
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			subs = append(subs, s)
		}
		cur.Reset()
	}
	runes := []rune(command)
	var quote rune
	var heredocs []heredoc
	pendingOperator := false
	wordStart := true
	var prev rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		atWordStart, afterRedirect := wordStart, prev == '>' || prev == '<'
		wordStart, prev = false, 0
		switch {
		case quote == '\'':
			cur.WriteRune(r)
			if r == '\'' {
				quote = 0
			}
			continue
		case r == '\\' && i+1 < len(runes) && runes[i+1] == '\n':
			i++
			wordStart = atWordStart
			continue
		case r == '\\' && i+1 < len(runes):
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i++
			continue
		case aware && r == '#' && quote == 0 && atWordStart:
			for i+1 < len(runes) && runes[i+1] != '\n' {
				i++
			}
			continue
		case r == '<' && quote == 0 && hasPrefixAt(runes, i, "<<<"):
			cur.WriteString("<<<")
			i += 2
			continue
		case aware && r == '<' && quote == 0 && hasPrefixAt(runes, i, "<<"):
			doc, end, docOK := readHeredoc(runes, i+2)
			if !docOK {
				return nil, false
			}
			heredocs = append(heredocs, doc)
			cur.WriteString(string(runes[i:end]))
			i = end - 1
			continue
		case r == '\'' && quote == 0:
			quote = r
			cur.WriteRune(r)
			continue
		case r == '"':
			if quote == '"' {
				quote = 0
			} else {
				quote = r
			}
			cur.WriteRune(r)
			continue
		case r == '`':
			end := indexRune(runes, i+1, '`')
			if end < 0 {
				return nil, false
			}
			inner, innerOK := splitBacktick(runes[i+1:end], quote == '"', aware)
			if !innerOK {
				return nil, false
			}
			subs = append(subs, inner...)
			writeSubstitution(&cur, runes[i+1:end])
			i = end
			continue
		case r == '$' && i+1 < len(runes) && runes[i+1] == '(':
			end := closingParen(runes, i+2)
			if end < 0 {
				return nil, false
			}
			inner, innerOK := splitBashAs(string(runes[i+2:end]), aware)
			if !innerOK {
				return nil, false
			}
			subs = append(subs, inner...)
			writeSubstitution(&cur, runes[i+2:end])
			i = end
			continue
		case quote == '"':
			cur.WriteRune(r)
			continue
		case r == '\n' && len(heredocs) > 0:
			flush()
			wordStart, pendingOperator = true, false
			bodies, next, bodyOK := readHeredocBodies(runes, i+1, heredocs)
			if !bodyOK {
				return nil, false
			}
			subs = append(subs, bodies...)
			heredocs = nil
			i = next - 1
			continue
		case isRedirectionPart(runes, i, afterRedirect):
		case r == '&' || r == '|' || r == ';' || r == '\n':
			flush()
			wordStart = true
			pendingOperator = r == '&' && i+1 < len(runes) && runes[i+1] == '&' ||
				r == '|' && i+1 < len(runes) && runes[i+1] == '|'
			if i+1 < len(runes) && (runes[i+1] == '&' || runes[i+1] == '|') {
				i++
			}
			continue
		}
		if strings.ContainsRune(" \t()", r) {
			wordStart = true
		}
		if !unicode.IsSpace(r) {
			pendingOperator = false
		}
		prev = r
		cur.WriteRune(r)
	}
	if quote != 0 || pendingOperator {
		return nil, false
	}
	flush()
	return subs, true
}

// splitBacktick splits the text of a backtick substitution. A " that
// splitBash reads as opening double quotes may sit in a here-document
// body or a comment, so both readings are checked.
func splitBacktick(text []rune, inDoubleQuotes, aware bool) ([]string, bool) {
	subs, ok := splitBashAs(unescapeBackticks(text, false), aware)
	if !ok || !inDoubleQuotes {
		return subs, ok
	}
	quoted, ok := splitBashAs(unescapeBackticks(text, true), aware)
	return append(subs, quoted...), ok
}

// substitutionOutput stands in for what a substitution prints. It is a
// character no command name holds, so a computed name can be told apart.
const substitutionOutput = "\uFFFC"

// writeSubstitution stands in for a substitution's output. An empty one
// expands to nothing, so the word after it can still be the command.
func writeSubstitution(cur *strings.Builder, text []rune) {
	if strings.TrimSpace(string(text)) != "" {
		cur.WriteString(substitutionOutput)
	}
}

// isRedirectionPart reports whether the & or | at i belongs to a
// redirection, such as `2>&1`, `&>file`, or `>|file`, not an operator.
// afterRedirect is whether an unescaped < or > comes right before it.
func isRedirectionPart(runes []rune, i int, afterRedirect bool) bool {
	switch runes[i] {
	case '&':
		return afterRedirect || i+1 < len(runes) && runes[i+1] == '>'
	case '|':
		return afterRedirect && runes[i-1] == '>'
	}
	return false
}

func hasPrefixAt(runes []rune, i int, prefix string) bool {
	return strings.HasPrefix(string(runes[i:min(len(runes), i+len(prefix))]), prefix)
}

// heredoc is a pending `<<` redirection: its body starts on the next
// line. A quoted delimiter turns off substitutions in the body.
type heredoc struct {
	delimiter         string
	stripTabs, expand bool
}

// readHeredoc reads the delimiter word after `<<` or `<<-` at from, and
// returns the index after it.
func readHeredoc(runes []rune, from int) (heredoc, int, bool) {
	doc := heredoc{expand: true}
	i := from
	if i < len(runes) && runes[i] == '-' {
		doc.stripTabs = true
		i++
	}
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	var word strings.Builder
	var quote rune
word:
	for ; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, doc.expand = r, false
		case r == '\\' && i+1 < len(runes):
			doc.expand = false
			i++
			word.WriteRune(runes[i])
		case unicode.IsSpace(r) || strings.ContainsRune(";&|<>()", r):
			break word
		default:
			word.WriteRune(r)
		}
	}
	if quote != 0 || word.Len() == 0 {
		return heredoc{}, 0, false
	}
	doc.delimiter = word.String()
	return doc, i, true
}

// readHeredocBodies reads the body of each pending here-document from
// from, one after the other, and returns the commands inside the
// substitutions of the unquoted ones and the index after the last
// delimiter line. A body with no delimiter line runs to the end, as
// bash reads it.
func readHeredocBodies(runes []rune, from int, docs []heredoc) ([]string, int, bool) {
	var subs []string
	i := from
	for _, doc := range docs {
		var body []rune
		for i < len(runes) {
			end := i
			for end < len(runes) && runes[end] != '\n' {
				end++
			}
			line := string(runes[i:end])
			i = min(end+1, len(runes))
			if doc.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			if line == doc.delimiter {
				break
			}
			body = append(body, []rune(line+"\n")...)
		}
		if !doc.expand {
			continue
		}
		inner, ok := heredocSubstitutions(body)
		if !ok {
			return nil, 0, false
		}
		subs = append(subs, inner...)
	}
	return subs, i, true
}

// heredocSubstitutions returns the commands inside the `$()` and
// backtick substitutions of an unquoted here-document body, where quotes
// are plain text.
func heredocSubstitutions(body []rune) ([]string, bool) {
	var subs []string
	for i := 0; i < len(body); i++ {
		switch {
		case body[i] == '\\':
			i++
		case body[i] == '`':
			end := indexRune(body, i+1, '`')
			if end < 0 {
				return nil, false
			}
			inner, ok := splitBacktick(body[i+1:end], true, true)
			if !ok {
				return nil, false
			}
			subs = append(subs, inner...)
			i = end
		case body[i] == '$' && i+1 < len(body) && body[i+1] == '(':
			end := closingParen(body, i+2)
			if end < 0 {
				return nil, false
			}
			inner, ok := splitBashAs(string(body[i+2:end]), true)
			if !ok {
				return nil, false
			}
			subs = append(subs, inner...)
			i = end
		}
	}
	return subs, true
}

func indexRune(runes []rune, from int, want rune) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == '\\' {
			i++
			continue
		}
		if runes[i] == want {
			return i
		}
	}
	return -1
}

// unescapeBackticks drops the backslash before `, \, and $, and before
// " when the substitution sits in double quotes, as bash does before it
// runs the text, so an escaped backtick opens a nested substitution.
func unescapeBackticks(runes []rune, inDoubleQuotes bool) string {
	escaped := "`\\$"
	if inDoubleQuotes {
		escaped += `"`
	}
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune(escaped, runes[i+1]) {
			i++
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

func closingParen(runes []rune, from int) int {
	depth := 1
	var quote rune
	for i := from; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// pathRuleMatches matches a Read or Edit path rule, gitignore style:
// `//path` from the filesystem root, `~/path` from home, `/path` from the
// project root, where Claude Code anchors project settings, and `path` or
// `./path` from the working directory, which is the project root here. A
// bare name matches at any depth; a single directory segment such as
// `src/**` matches only under the working directory, as an `if`
// condition does since Claude Code v2.1.214.
func pathRuleMatches(pattern, file, root string) bool {
	file = filepath.ToSlash(filepath.Clean(file))
	var anchor string
	switch {
	case strings.HasPrefix(pattern, "//"):
		anchor, pattern = "/", strings.TrimPrefix(pattern, "//")
	case strings.HasPrefix(pattern, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		anchor, pattern = filepath.ToSlash(home), strings.TrimPrefix(pattern, "~/")
	case strings.HasPrefix(pattern, "/"):
		anchor, pattern = filepath.ToSlash(root), strings.TrimPrefix(pattern, "/")
	default:
		anchor, pattern = filepath.ToSlash(root), strings.TrimPrefix(pattern, "./")
		if !strings.Contains(strings.TrimSuffix(pattern, "/"), "/") {
			pattern = "**/" + pattern
		}
	}
	rel, ok := strings.CutPrefix(file, strings.TrimSuffix(anchor, "/")+"/")
	if !ok {
		return false
	}
	return gitignoreRegexp(pattern).MatchString(rel)
}

func gitignoreRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case pattern[i] == '*':
			b.WriteString("[^/]*")
		case pattern[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString("(?:/.*)?$")
	return regexp.MustCompile(b.String())
}
