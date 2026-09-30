package markdown

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var commonMark = parser.NewParser(
	parser.WithBlockParsers(parser.DefaultBlockParsers()...),
	parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
)

type Heading struct {
	Start, End, Level int
	hardBreaks        map[int]int
	codeTrailing      map[int]bool
	contentStarts     map[int]int
}

func Headings(body string) []Heading {
	lines := strings.Split(body, "\n")
	offsets := lineOffsets(lines)
	doc := commonMark.Parse(text.NewReader([]byte(body)))
	var headings []Heading
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		h, ok := node.(*ast.Heading)
		if !ok {
			continue
		}
		start := lineAt(offsets, h.Pos())
		heading := Heading{Start: start, End: start, Level: h.Level}
		if !atxHeading.MatchString(strings.TrimSuffix(lines[start], "\r")) {
			heading.End = lineAt(offsets, h.Lines().At(h.Lines().Len()-1).Stop-1) + 1
			heading.contentStarts = map[int]int{}
			for i := 0; i < h.Lines().Len(); i++ {
				segment := h.Lines().At(i)
				line := lineAt(offsets, segment.Start)
				heading.contentStarts[line] = segment.Start - offsets[line]
			}
		}
		_ = ast.Walk(h, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if code, ok := node.(*ast.CodeSpan); entering && ok {
				if last, ok := code.LastChild().(*ast.Text); ok {
					firstLine, lastLine := lineAt(offsets, code.Pos()), lineAt(offsets, last.Segment.Stop-1)
					if firstLine < lastLine {
						if heading.codeTrailing == nil {
							heading.codeTrailing = map[int]bool{}
						}
						for i := firstLine; i < lastLine; i++ {
							heading.codeTrailing[i] = true
						}
					}
				}
			}
			if t, ok := node.(*ast.Text); entering && ok && t.HardLineBreak() {
				if heading.hardBreaks == nil {
					heading.hardBreaks = map[int]int{}
				}
				i := lineAt(offsets, t.Segment.Stop)
				heading.hardBreaks[i] = t.Segment.Stop - offsets[i]
			}
			return ast.WalkContinue, nil
		})
		headings = append(headings, heading)
	}
	return headings
}

func NestHeadings(body string, parent int) (string, int) {
	headings := Headings(body)
	shallowest := parent + 1
	for _, heading := range headings {
		shallowest = min(shallowest, heading.Level)
	}
	shift := parent + 1 - shallowest
	if shift == 0 {
		return body, 0
	}
	return shiftHeadings(strings.Split(body, "\n"), headings, shift), shift
}

func ShiftHeadings(body string, shift int) string {
	if shift == 0 {
		return body
	}
	return shiftHeadings(strings.Split(body, "\n"), Headings(body), shift)
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
		var title strings.Builder
		for i := heading.Start; i < heading.End; i++ {
			if i > heading.Start {
				if _, hard := heading.hardBreaks[i-1]; !hard {
					title.WriteByte(' ')
				}
			}
			line := lines[i]
			if stop, hard := heading.hardBreaks[i]; hard {
				title.WriteString(strings.Trim(line[heading.contentStarts[i]:stop], " \t\r"))
				title.WriteString("<br>")
			} else {
				line = strings.TrimSuffix(line[heading.contentStarts[i]:], "\r")
				if !heading.codeTrailing[i] {
					line = strings.TrimRight(line, " \t")
				}
				title.WriteString(line)
			}
		}
		content := title.String()
		closing := len(strings.TrimRight(content, "#"))
		if closing < len(content) && (closing == 0 || content[closing-1] == ' ' || content[closing-1] == '\t') {
			content = content[:closing] + "\\" + content[closing:]
		}
		ending := ""
		if strings.HasSuffix(lines[heading.End], "\r") {
			ending = "\r"
		}
		lines[heading.Start] = line[:indent] + marks + " " + content + ending
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

// LeavesBlockOpen identifies bodies whose last block would consume the next section.
func LeavesBlockOpen(body string) bool {
	source := []byte(body)
	doc := commonMark.Parse(text.NewReader(source))
	switch node := doc.LastChild().(type) {
	case *ast.FencedCodeBlock:
		start := node.Pos()
		marker := source[start]
		run := 0
		for start+run < len(source) && source[start+run] == marker {
			run++
		}
		end := strings.IndexByte(body[start:], '\n')
		if end < 0 {
			return true
		}
		end += start + 1
		if node.Lines().Len() > 0 {
			end = node.Lines().At(node.Lines().Len() - 1).Stop
		}
		return end >= len(body) || !strings.HasPrefix(strings.TrimSpace(strings.SplitN(body[end:], "\n", 2)[0]), strings.Repeat(string(marker), run))
	case *ast.HTMLBlock:
		if node.HTMLBlockType >= ast.HTMLBlockType6 || node.HasClosure() {
			return false
		}
		if node.Lines().Len() > 1 {
			return true
		}
		segment := node.Lines().At(0)
		line := string(segment.Value(source))
		switch node.HTMLBlockType {
		case ast.HTMLBlockType1:
			return !rawTextEnd.MatchString(line)
		case ast.HTMLBlockType2:
			return !strings.Contains(line, "-->")
		case ast.HTMLBlockType3:
			return !strings.Contains(line, "?>")
		case ast.HTMLBlockType4:
			return !strings.Contains(line, ">")
		case ast.HTMLBlockType5:
			return !strings.Contains(line, "]]>")
		}
	}
	return false
}

var (
	rawTextEnd = regexp.MustCompile(`(?i)</(?:pre|script|style|textarea)>`)
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)
)

func lineOffsets(lines []string) []int {
	offsets := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		offsets[i] = offsets[i-1] + len(lines[i-1]) + 1
	}
	return offsets
}

func lineAt(offsets []int, pos int) int {
	return sort.Search(len(offsets), func(i int) bool { return offsets[i] > pos }) - 1
}
