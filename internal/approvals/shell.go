package approvals

// The gate's shell reader fails closed: unlike hookrun's splitter, which
// reads as much as it can so a hook runs when unsure, it refuses any text
// it cannot read word for word, and the gate then asks.

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxCommandBytes caps the shell text the gate reads; a longer command
// asks.
const maxCommandBytes = 16 << 10

// errUnreadable is shell text the gate does not read word for word.
var errUnreadable = errors.New("unreadable shell text")

// shellWord is one word of a command after quote removal.
type shellWord struct {
	text string
	// quoted is set when any part of the word was quoted or escaped.
	quoted bool
	// tilde is set when the word starts with an unquoted ~.
	tilde bool
}

// redirect is one redirection of a command: its direction and target.
// A duplication such as 2>&1 has no target.
type redirect struct {
	output bool
	target shellWord
	dup    bool
}

// Separators that end a command in a list.
const (
	sepAnd  = "&&"
	sepOr   = "||"
	sepSeq  = ";"
	sepPipe = "|"
	sepBg   = "&"
)

// simpleCommand is one command of a list: its leading assignments,
// words and redirections, whether it reads a pipe, and the separator
// after it ("" for the last command).
type simpleCommand struct {
	assigns   []shellWord
	words     []shellWord
	redirects []redirect
	piped     bool
	sep       string
}

// parseShell reads command as a list of simple commands joined by
// &&, ||, ;, |, & or newlines. It refuses any text whose words it
// cannot know without running the shell: expansions, substitutions,
// here-documents, comments, globs, subshells, groups, brace expansion,
// line continuations, zsh's clobber redirections, control characters,
// text that is not UTF-8, and text over maxCommandBytes.
// It reads bash and zsh alike only where they agree.
func parseShell(command string) ([]simpleCommand, error) {
	if len(command) > maxCommandBytes {
		return nil, unreadable("command too long")
	}
	if !utf8.ValidString(command) {
		return nil, unreadable("not UTF-8")
	}
	p := shellParser{runes: []rune(command)}
	return p.parse()
}

type shellParser struct {
	runes    []rune
	pos      int
	commands []simpleCommand
	cur      simpleCommand
	word     strings.Builder
	inWord   bool
	w        shellWord
	// pendingRedirect waits for the word that is its target.
	pendingRedirect *redirect
	// needCommand is set after an operator that wants a command next.
	needCommand bool
}

func unreadable(reason string) error {
	return fmt.Errorf("%w: %s", errUnreadable, reason)
}

func isControl(r rune) bool {
	return r == 0 || unicode.IsControl(r) && r != '\t' && r != '\n'
}

func (p *shellParser) parse() ([]simpleCommand, error) {
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if isControl(r) {
			return nil, unreadable("control character")
		}
		var err error
		switch {
		case r == ' ' || r == '\t':
			p.endWord()
			p.pos++
		case r == '\n' || r == ';':
			if r == ';' && p.peek(1) == ';' {
				return nil, unreadable("case syntax")
			}
			err = p.separator(sepSeq, false, true)
			p.pos++
		case r == '&':
			err = p.ampersand()
		case r == '|':
			err = p.pipe()
		case r == '<' || r == '>':
			err = p.redirection(r)
		case r == '\'':
			err = p.singleQuote()
		case r == '"':
			err = p.doubleQuote()
		case r == '\\':
			err = p.escape()
		case r == '$' || r == '`':
			return nil, unreadable("expansion or substitution")
		case r == '(' || r == ')' || r == '{' || r == '}':
			return nil, unreadable("group, subshell or brace expansion")
		case r == '#' && !p.inWord:
			return nil, unreadable("comment")
		case r == '~' && !p.inWord:
			p.inWord = true
			p.w.tilde = true
			p.word.WriteRune(r)
			p.pos++
		case r == '[' && !p.inWord && (p.peek(1) == ' ' || p.peek(1) == 0):
			// The test command [, not a pattern.
			p.inWord = true
			p.word.WriteRune(r)
			p.pos++
		case strings.ContainsRune("*?[^#", r):
			// ^ and # are patterns under zsh's extended globbing.
			return nil, unreadable("glob pattern")
		default:
			p.inWord = true
			p.word.WriteRune(r)
			p.pos++
		}
		if err != nil {
			return nil, err
		}
	}
	if err := p.finish(); err != nil {
		return nil, err
	}
	return p.commands, nil
}

func (p *shellParser) peek(offset int) rune {
	if p.pos+offset < len(p.runes) {
		return p.runes[p.pos+offset]
	}
	return 0
}

func (p *shellParser) endWord() {
	if !p.inWord {
		return
	}
	w := p.w
	w.text = p.word.String()
	p.word.Reset()
	p.inWord = false
	p.w = shellWord{}
	switch {
	case p.pendingRedirect != nil:
		r := *p.pendingRedirect
		r.target = w
		p.cur.redirects = append(p.cur.redirects, r)
		p.pendingRedirect = nil
	case len(p.cur.words) == 0 && isAssignment(w):
		p.cur.assigns = append(p.cur.assigns, w)
	default:
		p.cur.words = append(p.cur.words, w)
	}
}

func (p *shellParser) curEmpty() bool {
	return len(p.cur.words) == 0 && len(p.cur.assigns) == 0 && len(p.cur.redirects) == 0
}

// separator ends the current command with sep. An empty command is an
// error after an operator that wants one; optional allows it otherwise,
// as for a trailing ; or blank lines.
func (p *shellParser) separator(sep string, piped, optional bool) error {
	p.endWord()
	if p.pendingRedirect != nil {
		return unreadable("redirection without a target")
	}
	if p.curEmpty() {
		if p.needCommand || !optional {
			return unreadable("operator without a command")
		}
		return nil
	}
	p.cur.sep = sep
	p.commands = append(p.commands, p.cur)
	p.cur = simpleCommand{piped: piped}
	p.needCommand = sep == sepAnd || sep == sepOr || sep == sepPipe
	return nil
}

func (p *shellParser) finish() error {
	p.endWord()
	if p.pendingRedirect != nil {
		return unreadable("redirection without a target")
	}
	if p.curEmpty() {
		if p.needCommand {
			return unreadable("operator without a command after it")
		}
		if n := len(p.commands); n > 0 && p.commands[n-1].sep == sepSeq {
			p.commands[n-1].sep = ""
		}
		return nil
	}
	p.commands = append(p.commands, p.cur)
	return nil
}

func (p *shellParser) ampersand() error {
	switch p.peek(1) {
	case '&':
		p.pos += 2
		return p.separator(sepAnd, false, false)
	case '>':
		p.endWord()
		if p.pendingRedirect != nil {
			return unreadable("redirection without a target")
		}
		p.pos += 2
		if p.peek(0) == '>' {
			p.pos++
		}
		if p.peek(0) == '!' || p.peek(0) == '|' {
			return unreadable("clobber redirection")
		}
		p.pendingRedirect = &redirect{output: true}
		return nil
	case '|', '!':
		return unreadable("zsh background operator")
	}
	// A lone & runs the command in the background; the next one starts.
	p.pos++
	return p.separator(sepBg, false, false)
}

func (p *shellParser) pipe() error {
	switch p.peek(1) {
	case '|':
		p.pos += 2
		return p.separator(sepOr, false, false)
	case '&':
		p.pos += 2
		return p.separator(sepPipe, true, false)
	}
	p.pos++
	return p.separator(sepPipe, true, false)
}

func (p *shellParser) redirection(r rune) error {
	// Unquoted digits right before the operator name the descriptor.
	if p.inWord && !p.w.quoted && !p.w.tilde && allDigits(p.word.String()) {
		p.word.Reset()
		p.inWord = false
		p.w = shellWord{}
	} else {
		p.endWord()
	}
	if p.pendingRedirect != nil {
		return unreadable("redirection without a target")
	}
	next := p.peek(1)
	switch {
	case r == '<' && (next == '<' || next == '(' || next == '>'):
		return unreadable("here-document, process substitution or read-write redirection")
	case r == '>' && next == '(':
		return unreadable("process substitution")
	case next == '&':
		p.pos += 2
		if p.peek(0) == '-' {
			p.pos++
		} else {
			start := p.pos
			for p.pos < len(p.runes) && unicode.IsDigit(p.runes[p.pos]) {
				p.pos++
			}
			if p.pos == start {
				return unreadable("descriptor duplication onto a word")
			}
		}
		if !p.atWordEnd() {
			return unreadable("descriptor duplication onto a word")
		}
		p.cur.redirects = append(p.cur.redirects, redirect{output: r == '>', dup: true})
		return nil
	case r == '>' && (next == '>' || next == '|'):
		p.pos += 2
	default:
		p.pos++
	}
	if r == '>' && (p.peek(0) == '!' || p.peek(0) == '|') {
		return unreadable("clobber redirection")
	}
	p.pendingRedirect = &redirect{output: r == '>'}
	return nil
}

func (p *shellParser) atWordEnd() bool {
	if p.pos >= len(p.runes) {
		return true
	}
	return strings.ContainsRune(" \t\n;&|<>", p.runes[p.pos])
}

func (p *shellParser) singleQuote() error {
	end := -1
	for i := p.pos + 1; i < len(p.runes); i++ {
		if p.runes[i] == '\'' {
			end = i
			break
		}
	}
	if end < 0 {
		return unreadable("open quote")
	}
	p.inWord = true
	p.w.quoted = true
	for _, r := range p.runes[p.pos+1 : end] {
		if isControl(r) {
			return unreadable("control character")
		}
		p.word.WriteRune(r)
	}
	p.pos = end + 1
	return nil
}

func (p *shellParser) doubleQuote() error {
	p.inWord = true
	p.w.quoted = true
	for i := p.pos + 1; i < len(p.runes); i++ {
		r := p.runes[i]
		switch {
		case r == '"':
			p.pos = i + 1
			return nil
		case r == '$' || r == '`':
			return unreadable("expansion or substitution in double quotes")
		case isControl(r):
			return unreadable("control character")
		case r == '\\' && i+1 < len(p.runes):
			next := p.runes[i+1]
			if next == '\n' {
				return unreadable("line continuation")
			}
			if strings.ContainsRune("\"\\$`", next) {
				p.word.WriteRune(next)
				i++
				continue
			}
			p.word.WriteRune(r)
		default:
			p.word.WriteRune(r)
		}
	}
	return unreadable("open quote")
}

func (p *shellParser) escape() error {
	next := p.peek(1)
	if next == 0 || next == '\n' {
		return unreadable("line continuation or trailing backslash")
	}
	if isControl(next) {
		return unreadable("control character")
	}
	p.inWord = true
	p.w.quoted = true
	p.word.WriteRune(next)
	p.pos += 2
	return nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAssignment reports NAME=value or NAME+=value.
func isAssignment(w shellWord) bool {
	name, _, ok := strings.Cut(w.text, "=")
	name = strings.TrimSuffix(name, "+")
	if !ok || name == "" || w.tilde {
		return false
	}
	for i, r := range name {
		if r > unicode.MaxASCII || r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// assignmentName is the variable an assignment word sets.
func assignmentName(w shellWord) string {
	name, _, _ := strings.Cut(w.text, "=")
	return strings.TrimSuffix(name, "+")
}
