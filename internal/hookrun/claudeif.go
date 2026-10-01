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

func globMatch(pattern, value string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile("^" + strings.Join(parts, ".*") + "$").MatchString(value)
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
		if bashPatternMatches(pattern, stripBashPrefix(sub)) {
			return true
		}
	}
	return false
}

func bashPatternMatches(pattern, command string) bool {
	if globMatch(pattern, command) {
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

func stripBashPrefix(command string) string {
	for {
		command = strings.TrimSpace(command)
		if loc := leadingAssignment.FindStringIndex(command); loc != nil {
			command = command[loc[1]:]
			continue
		}
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
func splitBash(command string) (subs []string, ok bool) {
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			subs = append(subs, s)
		}
		cur.Reset()
	}
	runes := []rune(command)
	var quote rune
	pendingOperator := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '\'':
			cur.WriteRune(r)
			if r == '\'' {
				quote = 0
			}
			continue
		case r == '\\' && i+1 < len(runes):
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i++
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
			inner, innerOK := splitBash(string(runes[i+1 : end]))
			if !innerOK {
				return nil, false
			}
			subs = append(subs, inner...)
			cur.WriteString("_")
			i = end
			continue
		case r == '$' && i+1 < len(runes) && runes[i+1] == '(':
			end := closingParen(runes, i+2)
			if end < 0 {
				return nil, false
			}
			inner, innerOK := splitBash(string(runes[i+2 : end]))
			if !innerOK {
				return nil, false
			}
			subs = append(subs, inner...)
			cur.WriteString("_")
			i = end
			continue
		case quote == '"':
			cur.WriteRune(r)
			continue
		case r == '&' || r == '|' || r == ';' || r == '\n':
			flush()
			pendingOperator = r == '&' && i+1 < len(runes) && runes[i+1] == '&' ||
				r == '|' && i+1 < len(runes) && runes[i+1] == '|'
			if i+1 < len(runes) && (runes[i+1] == '&' || runes[i+1] == '|') {
				i++
			}
			continue
		}
		if !unicode.IsSpace(r) {
			pendingOperator = false
		}
		cur.WriteRune(r)
	}
	if quote != 0 || pendingOperator {
		return nil, false
	}
	flush()
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
