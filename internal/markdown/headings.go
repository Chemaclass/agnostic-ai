package markdown

import (
	"regexp"
	"strings"
)

var listMarker = regexp.MustCompile(`^(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]|$)`)

// Heading is a heading a Scanner found. Start and End index its first
// and last lines: the same line for an ATX heading, the paragraph and its
// underline for a Setext one.
type Heading struct{ Start, End, Level int }

// Scanner reads Markdown one line at a time and reports the headings
// outside fenced code, indented code, raw HTML, lists, and block quotes.
type Scanner struct {
	line           int
	fence          byte
	fenceLength    int
	html           htmlBlock
	paragraph      bool
	paragraphStart int
	// lazy is set in a list item or block quote: until a blank line or a
	// thematic break, a line may continue its paragraph, and no Setext
	// underline can end it.
	lazy bool
}

// Reset closes every open block, as at the start of a document.
func (s *Scanner) Reset() {
	*s = Scanner{line: s.line}
}

// Scan reads the next line and returns the heading it completes.
func (s *Scanner) Scan(line string) (Heading, bool) {
	i := s.line
	s.line++
	line = strings.TrimSuffix(line, "\r")
	if s.fence == 0 && s.html.consume(line) {
		s.paragraph = false
		return Heading{}, false
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || strings.HasPrefix(line[indent:], "\t") {
		s.paragraph = false
		return Heading{}, false
	}
	line = line[indent:]
	if s.fence != 0 {
		run := len(line) - len(strings.TrimLeft(line, string(s.fence)))
		if run >= s.fenceLength && strings.TrimSpace(line[run:]) == "" {
			s.fence = 0
		}
		return Heading{}, false
	}
	if len(line) > 0 && (line[0] == '`' || line[0] == '~') {
		run := len(line) - len(strings.TrimLeft(line, line[:1]))
		if run >= 3 && (line[0] == '~' || !strings.Contains(line[run:], "`")) {
			s.fence, s.fenceLength, s.paragraph = line[0], run, false
			return Heading{}, false
		}
	}
	if level := atxLevel(line); level > 0 {
		s.paragraph, s.lazy = false, false
		return Heading{i, i, level}, true
	}
	if s.lazy {
		s.paragraph = false
		if strings.TrimSpace(line) == "" || thematicBreak(line) {
			s.lazy = false
		}
		return Heading{}, false
	}
	if level := setextLevel(line); s.paragraph && level > 0 {
		s.paragraph = false
		return Heading{s.paragraphStart, i, level}, true
	}
	if listMarker.MatchString(line) || strings.HasPrefix(line, ">") {
		s.paragraph, s.lazy = false, true
		return Heading{}, false
	}
	if strings.TrimSpace(line) == "" || thematicBreak(line) {
		s.paragraph = false
	} else if !s.paragraph {
		s.paragraph, s.paragraphStart = true, i
	}
	return Heading{}, false
}

// NestHeadings moves every heading of body down by the same number of
// levels, so the shallowest one sits below a heading at level parent. It
// returns the body and the number of levels it moved.
func NestHeadings(body string, parent int) (string, int) {
	lines := strings.Split(body, "\n")
	headings := headingsOf(lines)
	shallowest := parent + 1
	for _, heading := range headings {
		shallowest = min(shallowest, heading.Level)
	}
	shift := parent + 1 - shallowest
	if shift == 0 {
		return body, 0
	}
	return shiftHeadings(lines, headings, shift), shift
}

// ShiftHeadings moves every heading of body by shift levels, a negative
// shift moving them up, within the six levels Markdown has.
func ShiftHeadings(body string, shift int) string {
	if shift == 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	return shiftHeadings(lines, headingsOf(lines), shift)
}

func headingsOf(lines []string) []Heading {
	var scan Scanner
	var headings []Heading
	for _, line := range lines {
		if heading, ok := scan.Scan(line); ok {
			headings = append(headings, heading)
		}
	}
	return headings
}

func shiftHeadings(lines []string, headings []Heading, shift int) string {
	removed := map[int]bool{}
	for _, heading := range headings {
		line := lines[heading.Start]
		indent := len(line) - len(strings.TrimLeft(line, " "))
		marks := strings.Repeat("#", min(6, max(1, heading.Level+shift)))
		if heading.Start == heading.End {
			lines[heading.Start] = line[:indent] + marks + line[indent+heading.Level:]
			continue
		}
		text := make([]string, 0, heading.End-heading.Start)
		for i := heading.Start; i < heading.End; i++ {
			text = append(text, strings.TrimSpace(lines[i]))
		}
		ending := ""
		if strings.HasSuffix(lines[heading.End], "\r") {
			ending = "\r"
		}
		lines[heading.Start] = line[:indent] + marks + " " + strings.Join(text, " ") + ending
		for i := heading.Start + 1; i <= heading.End; i++ {
			removed[i] = true
		}
	}
	result := lines[:0]
	for i, line := range lines {
		if !removed[i] {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func atxLevel(line string) int {
	level := len(line) - len(strings.TrimLeft(line, "#"))
	if level == 0 || level > 6 || level < len(line) && line[level] != ' ' && line[level] != '\t' {
		return 0
	}
	return level
}

func setextLevel(line string) int {
	underline := strings.TrimRight(line, " \t")
	if underline == "" || strings.Trim(underline, underline[:1]) != "" {
		return 0
	}
	switch underline[0] {
	case '=':
		return 1
	case '-':
		return 2
	}
	return 0
}

func thematicBreak(line string) bool {
	line = strings.Join(strings.Fields(line), "")
	return len(line) >= 3 && strings.ContainsAny(line[:1], "-*_") && strings.Trim(line, line[:1]) == ""
}
