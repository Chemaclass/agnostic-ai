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
	"unicode/utf8"
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

// bashRuleRuns follows the hook `if` matcher of Claude Code 2.1.328, the
// Bash tool's preparePermissionMatcher. It reads command as a list of
// commands, each as its words with quotes, redirections, and leading
// `VAR=value` assignments removed, and matches the rule from the first
// word, also as `xargs <rule>`. It strips no wrapper. A command it
// cannot read runs the hook. A command it reads through a substitution
// or variable is matched only by a rule of one literal word and `*`.
func bashRuleRuns(pattern, command string) bool {
	reading, ok := readBash(command)
	if !ok {
		return true
	}
	prefix, isPrefix := strings.CutSuffix(pattern, ":*")
	isPrefix = isPrefix && prefix != ""
	if reading.nested && (isPrefix && strings.ContainsAny(prefix, " \t\n\r\f\v") || !isPrefix && !oneWordRule.MatchString(pattern)) {
		return true
	}
	if isPrefix {
		for _, c := range reading.commands {
			for _, p := range []string{prefix, "xargs " + prefix} {
				if c == p || strings.HasPrefix(c, p+" ") {
					return true
				}
			}
		}
		return false
	}
	rule, xargsRule := claudeWildcard(pattern), claudeWildcard("xargs "+pattern)
	for _, c := range reading.commands {
		c = blankRun.ReplaceAllString(c, " ")
		if rule != nil && rule.MatchString(c) || xargsRule != nil && xargsRule.MatchString(c) {
			return true
		}
	}
	return false
}

var (
	oneWordRule = regexp.MustCompile(`^[^\s*?\[]+\s?\*$`)
	blankRun    = regexp.MustCompile(`[ \t]+`)
)

// claudeWildcard compiles a Bash rule as Claude Code reads it, to match a
// command whose blank runs are one space: a run of blanks is one space,
// `*` matches anything, `\*` and `\\` are a literal star and backslash,
// `/**/` also matches `/`, and a trailing ` *` that is the only wildcard
// also matches the bare command. A rule holding a NUL matches nothing.
func claudeWildcard(pattern string) *regexp.Regexp {
	if strings.Contains(pattern, "\x00") {
		return nil
	}
	pattern = blankRun.ReplaceAllString(strings.TrimSpace(pattern), " ")
	var b strings.Builder
	stars := 0
	for i := 0; i < len(pattern); i++ {
		switch {
		case pattern[i] == '\\' && i+1 < len(pattern) && (pattern[i+1] == '*' || pattern[i+1] == '\\'):
			i++
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		case pattern[i] == '/' && strings.HasPrefix(pattern[i+1:], "**/"):
			for strings.HasPrefix(pattern[i+1:], "**/") {
				i += 3
				stars += 2
			}
			b.WriteString("/(?:.*/)?")
		case pattern[i] == '*':
			stars++
			b.WriteString(".*")
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	re := b.String()
	if bare, ok := strings.CutSuffix(re, " .*"); ok && stars == 1 {
		re = bare + "( .*)?"
	}
	return regexp.MustCompile("(?s)^" + re + "$")
}

// bashReading is what Claude Code reads from a Bash command. Each command
// is its words joined by spaces. nested is set when the command holds a
// substitution or variable that Claude Code does not read in place; then
// only a one-word rule is checked, so each command keeps only its name
// and whether more words follow.
type bashReading struct {
	commands []string
	nested   bool
}

const (
	// Claude Code writes these in a word for a substitution or an
	// arithmetic expansion it reads in place.
	substitutionOutput = "__CMDSUB_OUTPUT__"
	arithmeticOutput   = "__TRACKED_VAR__"
	// maxNestedLength is the longest command Claude Code reads through a
	// substitution.
	maxNestedLength = 10000
	maxBashDepth    = maxNestedLength / 2
)

var (
	controlCharacter = regexp.MustCompile(`[\x00-\x08\x0B-\x1F\x7F]`)
	unicodeBlank     = regexp.MustCompile(`[\x{00A0}\x{1680}\x{2000}-\x{200B}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}]`)
	// escapedBlank is Claude Code's test for backslash-escaped whitespace.
	// A line continuation after a blank is not one.
	escapedBlank   = regexp.MustCompile(`\\[ \t]|(?:^|[^ \t\\])(?:\\\\)*\\\n|[ \t](?:\\\\)+\\\n|\\[ \t]\\\n`)
	fdVariable     = regexp.MustCompile(`^\{[A-Za-z_][A-Za-z0-9_]*\}$`)
	assignmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?$`)
	plainName      = regexp.MustCompile(`^[\w./+-]+$`)
	literalNumbers = regexp.MustCompile(`^[0-9+\-*/%<>=!&|^~()?:, \t]*$`)
	// catHeredoc is the `"$(cat <<'EOF' ... EOF )"` form whose body Claude
	// Code reads as plain text.
	catHeredoc = regexp.MustCompile(`^\$\(cat <<[ \t]*(?:'([^'\n]+)'|"([^"\n]+)")\n`)
)

// bashDelimiters end an unquoted word.
const bashDelimiters = " \t\n;&|()<>"

// readBash returns what Claude Code reads from command, and false where
// it cannot read it and so runs the hook.
func readBash(command string) (bashReading, bool) {
	if controlCharacter.MatchString(command) || unicodeBlank.MatchString(command) ||
		escapedBlank.MatchString(command) || strings.Contains(command, "~[") {
		return bashReading{}, false
	}
	p := &bashParser{src: []rune(command), out: &bashOutput{}}
	top := p.parseList(0)
	switch {
	case p.out.unreadable:
		return bashReading{}, false
	case !p.out.nested:
		return bashReading{commands: p.out.argv}, true
	case !top.plain || p.out.unreadableNested || len(p.out.names) == 0 || len(command) > maxNestedLength:
		return bashReading{}, false
	}
	return bashReading{commands: p.out.names, nested: true}, true
}

// bashOutput collects what the parsers of one command read. argv holds
// each command's words; names holds each command's name, with a trailing
// space and `*` when more words follow. unreadableNested marks a command
// Claude Code cannot read through a substitution.
type bashOutput struct {
	argv, names                          []string
	nested, unreadableNested, unreadable bool
	heredocs                             int
}

type bashParser struct {
	src   []rune
	i     int
	depth int
	// inTestBracket is set while reading the words of `[` or `[[`.
	inTestBracket bool
	heredocs      []heredoc
	out           *bashOutput
}

// listShape describes a list of commands. plain is one command with no
// operator, redirection, assignment, keyword, or group: what Claude Code
// reads through a substitution.
type listShape struct {
	commands      int
	single, plain bool
}

type bashWord struct {
	text string
	// literal is an unquoted word with no backslash or expansion.
	literal    bool
	assignment bool
	// valueIsSubstitution is an assignment whose whole value is one
	// unquoted substitution, which Claude Code reads in place.
	valueIsSubstitution bool
	parts, variables    int
	hasOther            bool
	subs                []bashSubstitution
}

type bashSubstitution struct {
	shape  listShape
	quoted bool
	// besideText is a substitution in double quotes that hold other text,
	// which Claude Code reads in place.
	besideText bool
}

func (p *bashParser) fail() { p.out.unreadable = true }

func (p *bashParser) peek(offset int) rune {
	if p.i+offset < len(p.src) {
		return p.src[p.i+offset]
	}
	return 0
}

func (p *bashParser) hasPrefix(prefix string) bool { return hasPrefixAt(p.src, p.i, prefix) }

func hasPrefixAt(runes []rune, i int, prefix string) bool {
	if i >= len(runes) {
		return false
	}
	return strings.HasPrefix(string(runes[i:min(len(runes), i+len(prefix))]), prefix)
}

// skipBlanks skips spaces, tabs, and line continuations, and also
// newlines and comments when acrossLines is set.
func (p *bashParser) skipBlanks(acrossLines bool) {
	for p.i < len(p.src) && !p.out.unreadable {
		switch r := p.src[p.i]; {
		case r == ' ' || r == '\t':
			p.i++
		case r == '\\' && p.peek(1) == '\n':
			p.i += 2
		case r == '\n' && acrossLines:
			p.newline()
		case r == '#' && acrossLines:
			p.skipComment()
		default:
			return
		}
	}
}

func (p *bashParser) skipComment() {
	for p.i < len(p.src) && p.src[p.i] != '\n' {
		p.i++
	}
}

// newline reads a newline and the bodies of the here-documents that
// start after it. Claude Code reads no command in a quoted body.
func (p *bashParser) newline() {
	p.i++
	for _, doc := range p.heredocs {
		start, closed := p.i, false
		for p.i < len(p.src) && !closed {
			end := p.i
			for end < len(p.src) && p.src[end] != '\n' {
				end++
			}
			line := string(p.src[p.i:end])
			p.i = min(end+1, len(p.src))
			if doc.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			closed = line == doc.delimiter
		}
		// Claude Code's parser gives up on a body with no delimiter line
		// that holds shell syntax.
		if !closed && strings.ContainsAny(string(p.src[start:]), "$`(){}") {
			p.fail()
		}
	}
	p.heredocs = nil
}

// parseList reads commands up to end, `)` or 0 for the end of input, and
// leaves end unread.
func (p *bashParser) parseList(end rune) listShape {
	shape := listShape{single: true, plain: true}
	var open []string
	needCommand := false
	for !p.out.unreadable {
		p.skipBlanks(needCommand)
		if p.i >= len(p.src) || p.src[p.i] == ')' {
			// Claude Code reads an `if` or loop left open, but not a `for`
			// header with no body.
			if needCommand || slices.Contains(open, "for") || (p.i < len(p.src)) != (end == ')') {
				p.fail()
			}
			break
		}
		switch p.src[p.i] {
		case '\n':
			p.newline()
			continue
		case '#':
			p.skipComment()
			continue
		case ';', '&', '|':
			p.fail()
			continue
		}
		start := p.i
		count, plain := p.parseCommand(&open)
		shape.commands += count
		shape.plain = shape.plain && plain
		needCommand = false
		p.skipBlanks(false)
		switch {
		case p.i == start && p.i < len(p.src) && !strings.ContainsRune(";&|\n)", p.src[p.i]):
			p.fail()
		case p.hasPrefix(";;"):
			p.fail()
		case p.hasPrefix("&&"), p.hasPrefix("||"), p.hasPrefix("|&"):
			p.i += 2
			needCommand, shape.single = true, false
		case p.peek(0) == '|':
			p.i++
			needCommand, shape.single = true, false
		case p.peek(0) == ';', p.peek(0) == '&':
			// Claude Code cannot read `;` or `&` inside parentheses, nor `&`
			// inside an `if` or loop.
			if end == ')' || p.peek(0) == '&' && len(open) > 0 {
				p.fail()
			}
			p.i++
		}
	}
	shape.plain = shape.plain && shape.single && shape.commands == 1
	return shape
}

// parseCommand reads one command with its leading keywords, or a group
// in parentheses, and returns how many commands it holds and whether it
// is plain. open holds the `if` and loop keywords not yet closed.
func (p *bashParser) parseCommand(open *[]string) (int, bool) {
	plain := true
keywords:
	for !p.out.unreadable {
		p.skipBlanks(false)
		word := p.peekWord()
		top := ""
		if len(*open) > 0 {
			top = (*open)[len(*open)-1]
		}
		switch {
		case word == "!":
		case word == "if" || word == "while" || word == "until":
			*open = append(*open, word)
		case word == "then" && (top == "if" || top == "elif"),
			(word == "elif" || word == "else") && top == "then",
			word == "do" && (top == "while" || top == "until" || top == "for"):
			(*open)[len(*open)-1] = word
		case word == "fi" && (top == "then" || top == "else"), word == "done" && top == "do":
			*open = (*open)[:len(*open)-1]
			p.i += len(word)
			p.refuseRedirection()
			return 0, false
		case word == "for":
			p.i += len(word)
			p.parseForHeader()
			*open = append(*open, word)
			return 0, false
		case slices.Contains([]string{"then", "elif", "else", "do", "fi", "done", "case", "esac", "select", "function", "{", "}"}, word):
			// Claude Code cannot read these, or they do not parse here.
			p.fail()
			return 0, false
		default:
			break keywords
		}
		p.i += len(word)
		plain = false
	}
	if p.peek(0) == '(' {
		return p.parseSubshell()
	}
	var words []bashWord
	assignments, redirections := 0, 0
	afterRedirection, hereStringOnly := false, false
	for !p.out.unreadable {
		p.skipBlanks(false)
		if p.i >= len(p.src) || strings.ContainsRune(";|)\n", p.src[p.i]) || p.src[p.i] == '&' && p.peek(1) != '>' {
			break
		}
		if p.src[p.i] == '(' || afterRedirection && p.src[p.i] == '#' {
			p.fail()
			break
		}
		if p.src[p.i] == '#' {
			p.skipComment()
			break
		}
		if op, ok := p.parseRedirection(); ok {
			// Claude Code reads what follows a redirection after the
			// command name as more redirection targets, and gives up.
			if afterRedirection && op == "<<<" {
				p.fail()
			}
			plain = false
			redirections++
			hereStringOnly = redirections == 1 && op == "<<<"
			afterRedirection = afterRedirection || len(words) > 0 && op != "<<<" && op != "<<"
			continue
		}
		if afterRedirection {
			p.fail()
			break
		}
		p.inTestBracket = len(words) > 0 && (words[0].text == "[" || words[0].text == "[[")
		w := p.parseWord(len(words) == 0)
		p.inTestBracket = false
		p.record(w, !w.assignment)
		if w.assignment {
			assignments++
			plain = false
			continue
		}
		words = append(words, w)
	}
	switch {
	case p.out.unreadable:
		return 0, false
	case len(words) == 0 && assignments > 1:
		p.fail()
		return 0, false
	case len(words) == 0 && hereStringOnly:
		p.fail()
		return 0, false
	case len(words) == 0 && redirections > 0:
		p.out.argv = append(p.out.argv, "")
		return 0, false
	case len(words) == 0:
		return 0, false
	}
	argv := make([]string, len(words))
	for i, w := range words {
		argv[i] = w.text
	}
	// A name a substitution builds is decided at run time.
	if len(words[0].subs) > 0 || !claudeReadsArguments(argv) {
		p.fail()
		return 0, false
	}
	// Claude Code reads `[ x ]` as `[[ x` and `[[ x ]]` as `[[ x`.
	if last := len(argv) - 1; argv[0] == "[" || argv[0] == "[[" {
		argv[0], argv = "[[", argv[:last]
	}
	p.out.argv = append(p.out.argv, strings.Join(argv, " "))
	name := words[0]
	if len(words) > 1 {
		p.out.names = append(p.out.names, name.text+" *")
	} else {
		p.out.names = append(p.out.names, name.text)
	}
	if !name.literal || !plainName.MatchString(name.text) || claudeWrapper(name.text) {
		p.out.unreadableNested = true
	}
	return 1, plain
}

var (
	declarationOperand = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\+?=.*)?$`)
	claudeDeclarations = []string{"export", "readonly", "local", "declare", "typeset"}
)

// claudeReadsArguments reports whether Claude Code reads a command with
// these words. It refuses a test bracket left open, a declaration of
// something other than a plain name, a declaration flag that changes the
// value, and a pattern after `command`.
func claudeReadsArguments(argv []string) bool {
	last := argv[len(argv)-1]
	switch name := argv[0]; {
	case name == "[":
		return len(argv) > 1 && last == "]"
	case name == "[[":
		return len(argv) > 1 && last == "]]"
	case slices.Contains(claudeDeclarations, name):
		for _, operand := range argv[1:] {
			flag := strings.HasPrefix(operand, "-")
			if flag && (strings.HasPrefix(operand, "--") || name == "declare" || name == "typeset") || !flag && !declarationOperand.MatchString(operand) || tildeValue(operand) {
				return false
			}
		}
	case name == "command" && len(argv) > 1:
		return !strings.ContainsAny(argv[1], "*?[") && !slices.ContainsFunc(argv[2:], func(a string) bool { return strings.ContainsAny(a, "?[") })
	}
	return true
}

// tildeValue reports whether an assignment's value holds a `~`.
func tildeValue(operand string) bool {
	_, value, ok := strings.Cut(operand, "=")
	return ok && strings.Contains(value, "~")
}

// parseForHeader reads the name and words of a `for` loop. Claude Code
// reads a substitution that is a whole word there in place.
func (p *bashParser) parseForHeader() {
	p.skipBlanks(false)
	if p.hasPrefix("((") {
		p.fail()
		return
	}
	for !p.out.unreadable {
		p.skipBlanks(false)
		if p.i >= len(p.src) || strings.ContainsRune(";\n", p.src[p.i]) {
			return
		}
		if strings.ContainsRune("&|()<>#", p.src[p.i]) {
			p.fail()
			return
		}
		w := p.parseWord(false)
		if w.variables > 0 {
			p.fail()
		}
		for _, sub := range w.subs {
			if sub.quoted || w.parts != 1 || !sub.shape.single || sub.shape.commands != 1 {
				p.fail()
			}
		}
	}
}

// parseSubshell reads a group in parentheses. Two settings Claude Code
// 2.1.328 loads at run time change how it reads a command holding `(`,
// so a live session can differ here.
func (p *bashParser) parseSubshell() (int, bool) {
	if p.peek(1) == '(' || p.depth >= maxBashDepth {
		p.fail()
		return 0, false
	}
	p.i++
	p.depth++
	inner := p.parseList(')')
	p.depth--
	if p.out.unreadable {
		return 0, false
	}
	p.i++
	p.refuseRedirection()
	return inner.commands, false
}

// refuseRedirection fails on a redirection after a group or loop, which
// Claude Code cannot read.
func (p *bashParser) refuseRedirection() {
	p.skipBlanks(false)
	if op, _ := p.redirectionAt(p.i); op != "" {
		p.fail()
	}
}

// peekWord returns the text up to the next delimiter.
func (p *bashParser) peekWord() string {
	end := p.i
	for end < len(p.src) && !strings.ContainsRune(bashDelimiters, p.src[end]) {
		end++
	}
	return string(p.src[p.i:end])
}

// record notes how Claude Code reads the substitutions and variables of
// w. It reads a substitution in place when it is the whole value of an
// assignment or sits in double quotes beside other text of an argument,
// and reads the whole command through it otherwise.
func (p *bashParser) record(w bashWord, argument bool) {
	if w.variables > 0 {
		p.out.nested = true
		p.out.unreadableNested = p.out.unreadableNested || w.variables > 1 || w.hasOther
	}
	for _, sub := range w.subs {
		if sub.besideText && argument || w.valueIsSubstitution && !argument {
			if !sub.shape.single || sub.shape.commands != 1 {
				p.fail()
			}
			continue
		}
		p.out.nested = true
		p.out.unreadableNested = p.out.unreadableNested || !sub.shape.plain
	}
}

var redirectionOperators = []string{"<<<", "<<", "&>>", "&>", ">>", ">&", "<&", "<>", ">|", ">", "<"}

// redirectionAt returns the redirection operator at i, after an optional
// descriptor number, and the index after it, or "".
func (p *bashParser) redirectionAt(i int) (string, int) {
	j := i
	for j < len(p.src) && p.src[j] >= '0' && p.src[j] <= '9' {
		j++
	}
	for _, op := range redirectionOperators {
		if hasPrefixAt(p.src, j, op) && (j == i || op[0] != '&') {
			return op, j + len(op)
		}
	}
	return "", i
}

// parseRedirection reads a redirection and its target at the read
// position, and returns its operator. ok is false when there is none.
func (p *bashParser) parseRedirection() (op string, ok bool) {
	if word := p.peekWord(); fdVariable.MatchString(word) && strings.ContainsRune("<>", p.peek(len([]rune(word)))) {
		p.fail()
		return "", true
	}
	op, next := p.redirectionAt(p.i)
	if op == "" {
		return "", false
	}
	p.i = next
	switch {
	case op == "<>", p.peek(0) == '(' && (op == "<" || op == ">"):
		// A read-write redirection, or a process substitution.
		p.fail()
		return op, true
	case op == "<<":
		p.parseHeredoc()
		return op, true
	}
	p.skipBlanks(false)
	if p.i >= len(p.src) || strings.ContainsRune(bashDelimiters, p.src[p.i]) {
		p.fail()
		return op, true
	}
	target := p.parseWord(false)
	p.record(target, false)
	if (op == ">&" || op == "<&") && target.text == "-" {
		// Closing a descriptor.
		p.fail()
	}
	return op, true
}

// parseHeredoc reads the delimiter of a here-document. Claude Code cannot
// read an unquoted one, whose body bash expands, nor anything after the
// delimiter on its line.
func (p *bashParser) parseHeredoc() {
	doc, end, ok := readHeredoc(p.src, p.i)
	if !ok || doc.expand {
		p.fail()
		return
	}
	p.i = end
	for p.i < len(p.src) && (p.src[p.i] == ' ' || p.src[p.i] == '\t') {
		p.i++
	}
	if p.i < len(p.src) && p.src[p.i] != '\n' {
		p.fail()
		return
	}
	p.heredocs = append(p.heredocs, doc)
	p.out.heredocs++
}

// parseWord reads one word, with its quotes and backslashes removed.
func (p *bashParser) parseWord(commandPosition bool) bashWord {
	w := bashWord{literal: true}
	var text strings.Builder
	valueParts, valueSubs := 0, 0
	inLiteral, quotedParts := false, 0
	part := func(other bool) {
		valueParts++
		w.parts++
		w.hasOther = w.hasOther || other
		inLiteral = false
	}
	for p.i < len(p.src) && !p.out.unreadable {
		r := p.src[p.i]
		if strings.ContainsRune(bashDelimiters, r) {
			break
		}
		switch {
		case r == '=' && w.parts == 0 && !p.inTestBracket && (p.peek(1) == 0 || strings.ContainsRune(bashDelimiters, p.peek(1)) || isNameRune(p.peek(1), false) || strings.ContainsRune("./~-", p.peek(1))):
			// A lone `=` does not parse, and `=cmd` is a zsh expansion.
			p.fail()
			return w
		case r == '=' && commandPosition && !w.assignment && w.literal && assignmentName.MatchString(text.String()):
			w.assignment = true
			text.WriteRune(r)
			valueParts, valueSubs, inLiteral = 0, 0, false
			p.i++
			if p.peek(0) == '~' {
				p.fail()
			}
		case r == '[' && commandPosition && w.literal && assignmentName.MatchString(text.String()):
			// An array element assignment.
			p.fail()
			return w
		case r == '\'':
			end := slices.Index(p.src[p.i+1:], '\'')
			if end < 0 {
				p.fail()
				return w
			}
			end += p.i + 1
			text.WriteString(string(p.src[p.i+1 : end]))
			p.out.unreadableNested = true
			w.literal = false
			part(true)
			quotedParts++
			p.i = end + 1
		case r == '"':
			p.i++
			p.parseDoubleQuoted(&w, &text)
			w.literal = false
			part(true)
			quotedParts++
		case r == '\\' && p.peek(1) == '\n':
			p.i += 2
		case r == '\\':
			if p.i+1 >= len(p.src) {
				p.fail()
				return w
			}
			text.WriteRune(p.src[p.i+1])
			p.out.unreadableNested = true
			w.literal = false
			part(true)
			p.i += 2
		case r == '$' || r == '`':
			before := len(w.subs)
			variable := p.parseExpansion(&w, &text, false)
			substitution := len(w.subs) > before
			if substitution {
				valueSubs++
			}
			part(!variable && !substitution)
		default:
			text.WriteRune(r)
			if !inLiteral {
				part(true)
				inLiteral = true
			}
			p.i++
		}
	}
	w.text = text.String()
	// Claude Code cannot read a word with a `{` and a `,` or `..` unless
	// the whole word is quoted.
	if strings.Contains(w.text, "{") && (strings.Contains(w.text, ",") || strings.Contains(w.text, "..")) && quotedParts != w.parts {
		p.fail()
	}
	w.valueIsSubstitution = w.assignment && valueParts == 1 && valueSubs == 1
	return w
}

// parseDoubleQuoted reads up to the closing double quote.
func (p *bashParser) parseDoubleQuoted(w *bashWord, text *strings.Builder) {
	first := len(w.subs)
	hasText := false
	for p.i < len(p.src) && !p.out.unreadable {
		r := p.src[p.i]
		switch {
		case r == '"':
			p.i++
			for i := first; i < len(w.subs); i++ {
				w.subs[i].besideText = hasText
			}
			return
		case r == '\\' && p.peek(1) == '\n':
			p.i += 2
		case r == '\\' && strings.ContainsRune("$`\"\\", p.peek(1)):
			text.WriteRune(p.src[p.i+1])
			hasText = true
			p.i += 2
		case r == '$' && p.readCatHeredoc(text):
			hasText = true
		case r == '$' && p.hasPrefix("$(("):
			// Claude Code cannot read arithmetic in double quotes.
			p.fail()
		case r == '$' || r == '`':
			before, heredocs := len(w.subs), p.out.heredocs
			if p.parseExpansion(w, text, true) {
				// Claude Code cannot read a variable in double quotes.
				p.out.unreadableNested = true
			} else if len(w.subs) == before {
				hasText = true
			}
			if p.out.heredocs > heredocs {
				p.fail()
			}
		default:
			text.WriteRune(r)
			hasText = true
			p.i++
		}
	}
	p.fail()
}

// readCatHeredoc reads a `$(cat <<'EOF'` substitution whose here-document
// is the whole substitution, and writes its body as text, as Claude Code
// does. It reports whether it read one.
func (p *bashParser) readCatHeredoc(text *strings.Builder) bool {
	rest := string(p.src[p.i:min(len(p.src), p.i+maxNestedLength)])
	m := catHeredoc.FindStringSubmatch(rest)
	if m == nil {
		return false
	}
	delimiter := m[1] + m[2]
	for pos := len(m[0]); pos < len(rest); {
		eol := strings.IndexByte(rest[pos:], '\n')
		if eol < 0 {
			return false
		}
		if rest[pos:pos+eol] != delimiter {
			pos += eol + 1
			continue
		}
		after := strings.TrimLeft(rest[pos+eol+1:], " \t")
		if !strings.HasPrefix(after, ")") {
			return false
		}
		body := strings.TrimRight(rest[len(m[0]):pos], "\n")
		if strings.HasPrefix(body, "=") || strings.HasPrefix(body, "~") {
			p.fail()
		}
		text.WriteString(body)
		p.i += utf8.RuneCountInString(rest[:len(rest)-len(after)+1])
		return true
	}
	return false
}

// parseExpansion reads what starts with `$` or a backtick at the read
// position, and reports whether it is a variable.
func (p *bashParser) parseExpansion(w *bashWord, text *strings.Builder, quoted bool) bool {
	start := p.i
	w.literal = false
	if p.src[p.i] == '`' {
		sub := p.parseBacktick(quoted)
		sub.quoted = quoted
		w.subs = append(w.subs, sub)
		text.WriteString(substitutionOutput)
		return false
	}
	switch next := p.peek(1); {
	case next == '(' && p.peek(2) == '(':
		end := closingParen(p.src, p.i+3)
		// Claude Code reads arithmetic on literal numbers only.
		if end < 0 || end+1 >= len(p.src) || p.src[end+1] != ')' || !literalNumbers.MatchString(string(p.src[p.i+3:end])) {
			p.fail()
			return false
		}
		p.i = end + 2
		text.WriteString(arithmeticOutput)
		p.out.unreadableNested = true
		return false
	case next == '(':
		if p.depth >= maxBashDepth {
			p.fail()
			return false
		}
		p.i += 2
		p.depth++
		shape := p.parseList(')')
		p.depth--
		if p.out.unreadable {
			return false
		}
		p.i++
		w.subs = append(w.subs, bashSubstitution{shape: shape, quoted: quoted})
		text.WriteString(substitutionOutput)
		return false
	case isNameRune(next, true):
		p.i++
		for p.i < len(p.src) && isNameRune(p.src[p.i], false) {
			p.i++
		}
	case next != 0 && strings.ContainsRune("0123456789@*#?$!-", next):
		p.i += 2
	case quoted && next != '{':
		text.WriteRune('$')
		p.i++
		return false
	default:
		// A `${}` expansion, a `$''` or `$""` string, or a lone `$`.
		p.fail()
		return false
	}
	text.WriteString(string(p.src[start:p.i]))
	p.out.nested = true
	w.variables++
	return true
}

func isNameRune(r rune, first bool) bool {
	return r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || !first && r >= '0' && r <= '9'
}

// parseBacktick reads a backtick substitution. Inside it, \` \\ and \$,
// and \" in double quotes, lose their backslash before bash runs it.
func (p *bashParser) parseBacktick(quoted bool) bashSubstitution {
	end := -1
	for i := p.i + 1; i < len(p.src); i++ {
		if p.src[i] == '\\' {
			i++
			continue
		}
		if p.src[i] == '`' {
			end = i
			break
		}
	}
	// Claude Code does not read an escaped backtick or `$` inside
	// backticks.
	if body := string(p.src[p.i+1 : max(end, p.i+1)]); end < 0 || p.depth >= maxBashDepth || strings.Contains(body, "\\`") || strings.Contains(body, "\\$") {
		p.fail()
		return bashSubstitution{}
	}
	body := unescapeBackticks(p.src[p.i+1:end], quoted)
	p.i = end + 1
	if strings.TrimSpace(body) == "" {
		// Claude Code cannot tell where the words around an empty
		// substitution end.
		p.fail()
		return bashSubstitution{}
	}
	inner := &bashParser{src: []rune(body), depth: p.depth + 1, out: p.out}
	return bashSubstitution{shape: inner.parseList(0)}
}

// claudeWrapper reports whether Claude Code treats name as a command that
// runs another command, whose argument it cannot read through a
// substitution.
func claudeWrapper(name string) bool {
	base := strings.ToLower(name[strings.LastIndex(name, "/")+1:])
	if slices.Contains(claudeWrapperNames, base) {
		return true
	}
	base = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(base, ".exe"), ".bat"), ".cmd"), ".com")
	return slices.Contains(claudeWrapperNames, versionSuffix.ReplaceAllString(base, "$1"))
}

var versionSuffix = regexp.MustCompile(`([a-z])[\d.]+$`)

// claudeWrapperNames is the list Claude Code 2.1.328 checks before it
// reads a command through a substitution.
var claudeWrapperNames = []string{
	".", "[", "[[", "alias", "arch", "ash", "autoload", "awk", "bash", "bind", "break", "builtin", "bun", "busybox", "bye",
	"caffeinate", "chroot", "chrt", "cmd", "command", "compgen", "complete", "continue", "coproc", "csh", "dash", "declare",
	"deno", "disable", "doas", "emulate", "enable", "env", "eval", "exec", "exit", "expect", "export", "fakechroot",
	"fakeroot", "fc", "find", "firejail", "fish", "float", "flock", "foreach", "functions", "gawk", "getln", "getopts",
	"hash", "hush", "integer", "ionice", "jobs", "ksh", "let", "linux32", "linux64", "local", "logout", "ltrace", "lua",
	"mapfile", "mawk", "mksh", "nawk", "nice", "nocorrect", "node", "nodejs", "noglob", "nohup", "nsenter", "parallel",
	"pcre_match", "perl", "php", "pkexec", "powershell", "print", "printf", "private", "proot", "pwsh", "python",
	"python2", "python3", "rbash", "read", "readarray", "readonly", "repeat", "return", "rlwrap", "rsh", "ruby",
	"runuser", "sandbox-exec", "screen", "script", "set", "setarch", "setopt", "setpriv", "setsid", "sh", "shift",
	"shopt", "socat", "source", "stdbuf", "strace", "strftime", "su", "sudo", "syserror", "sysopen", "sysread",
	"sysseek", "systemd-run", "syswrite", "taskpolicy", "taskset", "tcsh", "test", "time", "timeout", "tmux", "toybox",
	"trap", "typeset", "unbuffer", "unset", "unsetopt", "unshare", "valgrind", "vared", "wait", "watch", "wsl", "xargs",
	"zcompile", "zcurses", "zf_chgrp", "zf_chmod", "zf_chown", "zf_ln", "zf_mkdir", "zf_mv", "zf_rm", "zf_rmdir",
	"zformat", "zftp", "zgetattr", "zlistattr", "zmodload", "zparseopts", "zpty", "zregexparse", "zselect", "zsh",
	"zsocket", "zstat", "zstyle", "zsystem", "ztcp", "ztie", "zuntie",
}

// heredoc is a pending `<<` redirection: its body starts on the next
// line. A quoted delimiter turns off expansion in the body.
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

// unescapeBackticks drops the backslash before `, \, and $, and before
// " when the substitution sits in double quotes.
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

// closingParen returns the index of the `)` that closes the one opened
// before from, or -1.
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
