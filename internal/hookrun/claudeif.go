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
// each command, outer and inner, is its text as written.
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
	maxBashDepth    = 1000
)

var (
	controlCharacter = regexp.MustCompile(`[\x00-\x08\x0B-\x1F\x7F]`)
	unicodeBlank     = regexp.MustCompile(`[\x{00A0}\x{1680}\x{2000}-\x{200B}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}]`)
	// escapedBlank is Claude Code's test for backslash-escaped whitespace.
	// A line continuation after a blank is not one.
	escapedBlank   = regexp.MustCompile(`\\[ \t]|(?:^|[^ \t\\])(?:\\\\)*\\\n|[ \t](?:\\\\)+\\\n|\\[ \t]\\\n`)
	braceExpansion = regexp.MustCompile(`\{[^{}]*(?:,|\.\.)[^{}]*\}`)
	fdVariable     = regexp.MustCompile(`^\{[A-Za-z_][A-Za-z0-9_]*\}$`)
	descriptor     = regexp.MustCompile(`^(?:[0-9]+|-)$`)
	assignmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?$`)
	plainName      = regexp.MustCompile(`^[\w./+-]+$`)
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
		return bashReading{commands: p.out.words}, true
	case !top.plain || p.out.unreadableNested || len(p.out.texts) == 0 || len(command) > maxNestedLength:
		return bashReading{}, false
	}
	return bashReading{commands: p.out.texts, nested: true}, true
}

// bashOutput collects what the parsers of one command read: words and
// texts hold each command as words and as written. unreadableNested
// marks text Claude Code cannot read through a substitution.
type bashOutput struct {
	words, texts                         []string
	nested, unreadableNested, unreadable bool
}

type bashParser struct {
	src      []rune
	i        int
	depth    int
	heredocs []heredoc
	out      *bashOutput
}

// listShape describes a list of commands. plain is one command with no
// operator, redirection, assignment, keyword, or group: what Claude Code
// reads through a substitution.
type listShape struct {
	commands      int
	single, plain bool
}

type bashWord struct {
	text, raw string
	// literal is an unquoted word with no backslash or expansion.
	literal    bool
	assignment bool
	// valueIsSubstitution is an assignment whose whole value is one
	// unquoted substitution, which Claude Code reads in place.
	valueIsSubstitution   bool
	hasVariable, hasOther bool
	subs                  []bashSubstitution
}

type bashSubstitution struct {
	shape listShape
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
	needCommand := false
	for !p.out.unreadable {
		p.skipBlanks(needCommand)
		if p.i >= len(p.src) || p.src[p.i] == ')' {
			if needCommand || (p.i < len(p.src)) != (end == ')') {
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
		count, plain := p.parseCommand()
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
			// Claude Code cannot read `;` or `&` inside parentheses.
			if end == ')' {
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
// is plain.
func (p *bashParser) parseCommand() (int, bool) {
	plain := true
keywords:
	for !p.out.unreadable {
		p.skipBlanks(false)
		switch word := p.peekWord(); word {
		case "!", "if", "then", "else", "elif", "do", "while", "until":
			p.i += len([]rune(word))
			plain = false
		case "fi", "done":
			p.i += len([]rune(word))
			return 0, false
		case "for", "select":
			// Claude Code reads no command in a loop's header.
			for p.i < len(p.src) && p.src[p.i] != ';' && p.src[p.i] != '\n' {
				p.i++
			}
			return 0, false
		case "case", "function", "{", "}":
			p.fail()
			return 0, false
		default:
			break keywords
		}
	}
	if p.peek(0) == '(' {
		return p.parseSubshell()
	}
	var words []bashWord
	afterFileRedirect := false
	for !p.out.unreadable {
		p.skipBlanks(false)
		if p.i >= len(p.src) || strings.ContainsRune(";|)\n", p.src[p.i]) || p.src[p.i] == '&' && p.peek(1) != '>' {
			break
		}
		if p.src[p.i] == '#' {
			p.skipComment()
			break
		}
		if p.src[p.i] == '(' {
			p.fail()
			break
		}
		if file, ok := p.parseRedirection(); ok {
			plain = false
			afterFileRedirect = afterFileRedirect || file && len(words) > 0
			continue
		}
		if afterFileRedirect {
			// Claude Code reads a word after a redirection that follows the
			// command name as another redirection target, and gives up.
			p.fail()
			break
		}
		w := p.parseWord(len(words) == 0)
		p.record(w, !w.assignment)
		if w.assignment {
			plain = false
			continue
		}
		words = append(words, w)
	}
	if p.out.unreadable || len(words) == 0 {
		return 0, false
	}
	argv, texts := make([]string, len(words)), make([]string, len(words))
	for i, w := range words {
		argv[i], texts[i] = w.text, w.raw
	}
	p.out.words = append(p.out.words, strings.Join(argv, " "))
	p.out.texts = append(p.out.texts, strings.Join(texts, " "))
	if name := words[0]; !name.literal || !plainName.MatchString(name.text) || claudeWrapper(name.text) {
		p.out.unreadableNested = true
	}
	return 1, plain
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
	for !p.out.unreadable {
		p.skipBlanks(false)
		if _, ok := p.parseRedirection(); !ok {
			break
		}
	}
	return inner.commands, false
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
	if w.hasVariable {
		p.out.nested = true
		p.out.unreadableNested = p.out.unreadableNested || w.hasOther
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

// parseRedirection reads a redirection and its target at the read
// position. ok is false when there is none. file is false for a
// here-string, a here-document, and a copy such as `2>&1`.
func (p *bashParser) parseRedirection() (file, ok bool) {
	if word := p.peekWord(); fdVariable.MatchString(word) && strings.ContainsRune("<>", p.peek(len([]rune(word)))) {
		p.fail()
		return false, true
	}
	j := p.i
	for j < len(p.src) && p.src[j] >= '0' && p.src[j] <= '9' {
		j++
	}
	var op string
	for _, o := range []string{"<<<", "<<", "&>>", "&>", ">>", ">&", "<&", "<>", ">|", ">", "<"} {
		if hasPrefixAt(p.src, j, o) && (j == p.i || o[0] != '&') {
			op = o
			break
		}
	}
	if op == "" {
		return false, false
	}
	p.i = j + len(op)
	switch {
	case p.peek(0) == '(' && (op == "<" || op == ">"):
		// A process substitution.
		p.fail()
		return false, true
	case op == "<<":
		doc, end, docOK := readHeredoc(p.src, p.i)
		// Claude Code cannot read an unquoted here-document, whose body
		// bash expands, nor one inside a substitution.
		if !docOK || doc.expand || p.depth > 0 {
			p.fail()
			return false, true
		}
		p.i = end
		p.heredocs = append(p.heredocs, doc)
		return false, true
	}
	p.skipBlanks(false)
	if p.i >= len(p.src) || strings.ContainsRune(bashDelimiters, p.src[p.i]) {
		p.fail()
		return false, true
	}
	target := p.parseWord(false)
	p.record(target, false)
	if op == ">&" || op == "<&" {
		return !descriptor.MatchString(target.text), true
	}
	return op != "<<<", true
}

// parseWord reads one word. text has its quotes and backslashes removed,
// raw is the word as Claude Code shows it when it reads through a
// substitution: double quotes removed, the rest as written.
func (p *bashParser) parseWord(commandPosition bool) bashWord {
	w := bashWord{literal: true}
	var text, raw, bare strings.Builder
	valueParts, valueSubs := 0, 0
	inLiteral := false
	part := func(other bool) {
		valueParts++
		w.hasOther = w.hasOther || other
		inLiteral = false
	}
	for p.i < len(p.src) && !p.out.unreadable {
		r := p.src[p.i]
		if strings.ContainsRune(bashDelimiters, r) {
			break
		}
		switch {
		case r == '=' && commandPosition && !w.assignment && w.literal && assignmentName.MatchString(text.String()):
			w.assignment = true
			text.WriteRune(r)
			raw.WriteRune(r)
			valueParts, valueSubs, inLiteral = 0, 0, false
			p.i++
		case r == '\'':
			end := slices.Index(p.src[p.i+1:], '\'')
			if end < 0 {
				p.fail()
				return w
			}
			end += p.i + 1
			text.WriteString(string(p.src[p.i+1 : end]))
			raw.WriteString(string(p.src[p.i : end+1]))
			p.out.unreadableNested = true
			w.literal = false
			part(true)
			p.i = end + 1
		case r == '"':
			p.i++
			p.parseDoubleQuoted(&w, &text, &raw)
			w.literal = false
			part(true)
		case r == '\\' && p.peek(1) == '\n':
			p.i += 2
		case r == '\\':
			if p.i+1 < len(p.src) {
				text.WriteRune(p.src[p.i+1])
				raw.WriteString(string(p.src[p.i : p.i+2]))
			}
			p.out.unreadableNested = true
			w.literal = false
			part(true)
			p.i += 2
		case r == '$' || r == '`':
			before := len(w.subs)
			variable := p.parseExpansion(&w, &text, &raw, false)
			substitution := len(w.subs) > before
			if substitution {
				valueSubs++
			}
			part(!variable && !substitution)
		default:
			text.WriteRune(r)
			raw.WriteRune(r)
			bare.WriteRune(r)
			if !inLiteral {
				part(true)
				inLiteral = true
			}
			p.i++
		}
	}
	if braceExpansion.MatchString(bare.String()) {
		p.fail()
	}
	w.text, w.raw = text.String(), raw.String()
	w.valueIsSubstitution = w.assignment && valueParts == 1 && valueSubs == 1
	return w
}

// parseDoubleQuoted reads up to the closing double quote.
func (p *bashParser) parseDoubleQuoted(w *bashWord, text, raw *strings.Builder) {
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
			raw.WriteString(string(p.src[p.i : p.i+2]))
			hasText = true
			p.i += 2
		case r == '$' || r == '`':
			before := len(w.subs)
			if p.parseExpansion(w, text, raw, true) {
				// Claude Code cannot read a variable in double quotes.
				p.out.unreadableNested = true
			} else if len(w.subs) == before {
				hasText = true
			}
		default:
			text.WriteRune(r)
			raw.WriteRune(r)
			hasText = true
			p.i++
		}
	}
	p.fail()
}

// parseExpansion reads what starts with `$` or a backtick at the read
// position, and reports whether it is a variable.
func (p *bashParser) parseExpansion(w *bashWord, text, raw *strings.Builder, quoted bool) bool {
	start := p.i
	w.literal = false
	if p.src[p.i] == '`' {
		w.subs = append(w.subs, p.parseBacktick(quoted))
		text.WriteString(substitutionOutput)
		raw.WriteString(string(p.src[start:p.i]))
		return false
	}
	variable := false
	switch next := p.peek(1); {
	case next == '(' && p.peek(2) == '(':
		end := closingParen(p.src, p.i+3)
		if end < 0 || end+1 >= len(p.src) || p.src[end+1] != ')' {
			p.fail()
			return false
		}
		p.i = end + 2
		text.WriteString(arithmeticOutput)
		p.out.unreadableNested = true
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
		w.subs = append(w.subs, bashSubstitution{shape: shape})
		text.WriteString(substitutionOutput)
	case next == '{', next == '\'' && !quoted:
		// A `${}` expansion, or a `$''` string with escape codes.
		p.fail()
		return false
	case next == '"' && !quoted:
		p.i++
		return false
	case isNameRune(next, true):
		p.i++
		for p.i < len(p.src) && isNameRune(p.src[p.i], false) {
			p.i++
		}
		variable = true
	case next != 0 && strings.ContainsRune("0123456789@*#?$!-", next):
		p.i += 2
		variable = true
	default:
		text.WriteRune('$')
		raw.WriteRune('$')
		p.i++
		return false
	}
	if variable {
		text.WriteString(string(p.src[start:p.i]))
		p.out.nested = true
		w.hasVariable = true
	}
	raw.WriteString(string(p.src[start:p.i]))
	return variable
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
	// Claude Code does not read a backtick escaped inside backticks.
	if end < 0 || p.depth >= maxBashDepth || strings.Contains(string(p.src[p.i+1:end]), "\\`") {
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
