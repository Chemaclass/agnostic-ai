package approvals

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// errUnreadable is shell text the gate does not read word for word.
var errUnreadable = errors.New("unreadable shell text")

// shellWord is one word of a command after quote removal.
type shellWord struct {
	text string
	// quoted is set when any part of the word was quoted or escaped.
	quoted bool
	// glob is set when the word holds an unquoted *, ? or [.
	glob bool
	// tilde is set when the word starts with an unquoted ~.
	tilde bool
}

// redirect is one redirection of a command: its operator and target.
// A duplication such as 2>&1 has no target.
type redirect struct {
	output bool
	target shellWord
	dup    bool
}

// simpleCommand is one command of a list: its leading assignments,
// words and redirections, and whether it reads a pipe.
type simpleCommand struct {
	assigns   []shellWord
	words     []shellWord
	redirects []redirect
	piped     bool
}

// parseShell reads command as a list of simple commands joined by
// &&, ||, ;, |, & or newlines. It refuses any text whose words it
// cannot know without running the shell: expansions, substitutions,
// here-documents, comments, subshells, groups, brace expansion, line
// continuations and control characters.
func parseShell(command string) ([]simpleCommand, error) {
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

func (p *shellParser) parse() ([]simpleCommand, error) {
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if r == 0 || (unicode.IsControl(r) && r != '\t' && r != '\n') {
			return nil, unreadable("control character")
		}
		var err error
		switch {
		case r == ' ' || r == '\t':
			err = p.endWord()
			p.pos++
		case r == '\n' || r == ';':
			if r == ';' && p.peek(1) == ';' {
				return nil, unreadable("case syntax")
			}
			err = p.endCommand(false, r == ';')
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
			p.startWord()
			p.w.tilde = true
			p.word.WriteRune(r)
			p.pos++
		case r == '*' || r == '?' || r == '[':
			p.startWord()
			p.w.glob = true
			p.word.WriteRune(r)
			p.pos++
		default:
			p.startWord()
			p.word.WriteRune(r)
			p.pos++
		}
		if err != nil {
			return nil, err
		}
	}
	if err := p.endCommand(true, false); err != nil {
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

func (p *shellParser) startWord() {
	p.inWord = true
}

func (p *shellParser) endWord() error {
	if !p.inWord {
		return nil
	}
	w := p.w
	w.text = p.word.String()
	if w.text == "[" {
		// The test command, not a pattern.
		w.glob = false
	}
	p.word.Reset()
	p.inWord = false
	p.w = shellWord{}
	if p.pendingRedirect != nil {
		r := *p.pendingRedirect
		r.target = w
		p.cur.redirects = append(p.cur.redirects, r)
		p.pendingRedirect = nil
		return nil
	}
	if len(p.cur.words) == 0 && isAssignment(w) {
		p.cur.assigns = append(p.cur.assigns, w)
		return nil
	}
	p.cur.words = append(p.cur.words, w)
	return nil
}

// endCommand closes the current command. last is set at the end of
// the text; optional says an empty command is fine there, as after a
// trailing ; or between blank lines.
func (p *shellParser) endCommand(last, optional bool) error {
	if err := p.endWord(); err != nil {
		return err
	}
	if p.pendingRedirect != nil {
		return unreadable("redirection without a target")
	}
	empty := len(p.cur.words) == 0 && len(p.cur.assigns) == 0 && len(p.cur.redirects) == 0
	if empty {
		if p.needCommand {
			return unreadable("operator without a command after it")
		}
		if !last && !optional && p.cur.piped {
			return unreadable("pipe without a command after it")
		}
		return nil
	}
	p.commands = append(p.commands, p.cur)
	p.cur = simpleCommand{}
	p.needCommand = false
	return nil
}

// operator closes the current command, which must not be empty, and
// expects one after it.
func (p *shellParser) operator(piped bool) error {
	if err := p.endWord(); err != nil {
		return err
	}
	if len(p.cur.words) == 0 && len(p.cur.assigns) == 0 && len(p.cur.redirects) == 0 {
		return unreadable("operator without a command before it")
	}
	if err := p.endCommand(false, false); err != nil {
		return err
	}
	p.cur.piped = piped
	p.needCommand = true
	return nil
}

func (p *shellParser) ampersand() error {
	switch p.peek(1) {
	case '&':
		p.pos += 2
		return p.operator(false)
	case '>':
		if err := p.endWord(); err != nil {
			return err
		}
		p.pos += 2
		if p.peek(0) == '>' {
			p.pos++
		}
		p.pendingRedirect = &redirect{output: true}
		return nil
	}
	// A lone & runs the command in the background; the next one starts.
	p.pos++
	if err := p.operator(false); err != nil {
		return err
	}
	p.needCommand = false
	return nil
}

func (p *shellParser) pipe() error {
	switch p.peek(1) {
	case '|':
		p.pos += 2
		return p.operator(false)
	case '&':
		p.pos += 2
		return p.operator(true)
	}
	p.pos++
	return p.operator(true)
}

func (p *shellParser) redirection(r rune) error {
	// Unquoted digits right before the operator name the descriptor.
	if p.inWord && !p.w.quoted && !p.w.glob && !p.w.tilde && allDigits(p.word.String()) {
		p.word.Reset()
		p.inWord = false
		p.w = shellWord{}
	} else if err := p.endWord(); err != nil {
		return err
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
			p.cur.redirects = append(p.cur.redirects, redirect{output: r == '>', dup: true})
			return nil
		}
		start := p.pos
		for p.pos < len(p.runes) && unicode.IsDigit(p.runes[p.pos]) {
			p.pos++
		}
		if p.pos == start || !p.atWordEnd() {
			return unreadable("descriptor duplication onto a word")
		}
		p.cur.redirects = append(p.cur.redirects, redirect{output: r == '>', dup: true})
		return nil
	case r == '>' && (next == '>' || next == '|'):
		p.pos += 2
	default:
		p.pos++
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
	p.startWord()
	p.w.quoted = true
	for _, r := range p.runes[p.pos+1 : end] {
		if r == 0 || (unicode.IsControl(r) && r != '\t' && r != '\n') {
			return unreadable("control character")
		}
		p.word.WriteRune(r)
	}
	p.pos = end + 1
	return nil
}

func (p *shellParser) doubleQuote() error {
	p.startWord()
	p.w.quoted = true
	for i := p.pos + 1; i < len(p.runes); i++ {
		r := p.runes[i]
		switch {
		case r == '"':
			p.pos = i + 1
			return nil
		case r == '$' || r == '`':
			return unreadable("expansion or substitution in double quotes")
		case r == 0 || (unicode.IsControl(r) && r != '\t' && r != '\n'):
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
	if unicode.IsControl(next) && next != '\t' {
		return unreadable("control character")
	}
	p.startWord()
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

// isAssignment reports NAME=value, where NAME is unquoted.
func isAssignment(w shellWord) bool {
	name, _, ok := strings.Cut(w.text, "=")
	if !ok || name == "" || w.glob || w.tilde {
		return false
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
		if r > unicode.MaxASCII {
			return false
		}
	}
	return !strings.ContainsAny(name, `'"\`)
}
