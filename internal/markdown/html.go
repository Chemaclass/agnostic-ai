package markdown

import (
	"regexp"
	"strings"
)

var (
	rawTextStart = regexp.MustCompile(`(?i)^<(?:pre|script|style|textarea)(?:[ \t>]|$)`)
	rawTextEnd   = regexp.MustCompile(`(?i)</(?:pre|script|style|textarea)>`)
	blockTag     = regexp.MustCompile(`(?i)^</?(?:address|article|aside|base|basefont|blockquote|body|caption|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption|figure|footer|form|frame|frameset|h[1-6]|head|header|hr|html|iframe|legend|li|link|main|menu|menuitem|nav|noframes|ol|optgroup|option|p|param|search|section|source|summary|table|tbody|td|tfoot|th|thead|title|tr|track|ul)(?:[ \t>]|/>|$)`)
	lineTag      = regexp.MustCompile(`^(?:<([A-Za-z][A-Za-z0-9-]*)(?:[ \t]+[A-Za-z_:][A-Za-z0-9_.:-]*(?:[ \t]*=[ \t]*(?:[^ \t"'=<>` + "`" + `]+|'[^']*'|"[^"]*"))?)*[ \t]*/?>|</([A-Za-z][A-Za-z0-9-]*)[ \t]*>)[ \t]*$`)
)

// htmlBlock tracks a raw HTML block by its CommonMark kind: 1 for pre,
// script, style, and textarea, 2 for comments, 3 for processing
// instructions, 4 for declarations, 5 for CDATA, 6 for block-level tags,
// and 7 for any other complete tag alone on its line. 0 is no block.
type htmlBlock struct {
	kind int
}

// consume reports whether line belongs to a raw HTML block, opening one
// when line starts it. A kind 7 tag cannot interrupt a paragraph.
func (b *htmlBlock) consume(line string, paragraph bool) bool {
	if b.kind != 0 {
		if htmlBlockEnds(b.kind, line) {
			b.kind = 0
		}
		return true
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || strings.HasPrefix(line[indent:], "\t") {
		return false
	}
	line = line[indent:]
	b.kind = htmlBlockStart(line, paragraph)
	if b.kind == 0 {
		return false
	}
	if b.kind <= 5 && htmlBlockEnds(b.kind, line) {
		b.kind = 0
	}
	return true
}

func htmlBlockStart(line string, paragraph bool) int {
	switch {
	case rawTextStart.MatchString(line):
		return 1
	case strings.HasPrefix(line, "<!--"):
		return 2
	case strings.HasPrefix(line, "<?"):
		return 3
	case len(line) > 2 && strings.HasPrefix(line, "<!") && line[2] >= 'A' && line[2] <= 'Z':
		return 4
	case strings.HasPrefix(line, "<![CDATA["):
		return 5
	case blockTag.MatchString(line):
		return 6
	case !paragraph && lineTagStart(line):
		return 7
	}
	return 0
}

func htmlBlockEnds(kind int, line string) bool {
	switch kind {
	case 1:
		return rawTextEnd.MatchString(line)
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return strings.TrimSpace(line) == ""
}

func lineTagStart(line string) bool {
	m := lineTag.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	switch strings.ToLower(m[1] + m[2]) {
	case "pre", "script", "style", "textarea":
		return false
	}
	return true
}
