package markdown

import (
	"regexp"
	"strings"
)

var htmlTag = regexp.MustCompile(`(?i)^</?([a-z][a-z0-9-]*)(?:[ \t/>]|$)`)

type htmlBlock struct {
	active bool
	end    string
}

func (b *htmlBlock) consume(line string) bool {
	if b.active {
		if b.end == "" && strings.TrimSpace(line) == "" || b.end != "" && strings.Contains(strings.ToLower(line), b.end) {
			b.active = false
		}
		return true
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || strings.HasPrefix(line[indent:], "\t") {
		return false
	}
	line = line[indent:]
	end, starts := htmlBlockStart(line)
	if !starts {
		return false
	}
	b.end = end
	b.active = end == "" || !strings.Contains(strings.ToLower(line), end)
	return true
}

func htmlBlockStart(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "<!--"):
		return "-->", true
	case strings.HasPrefix(line, "<?"):
		return "?>", true
	case strings.HasPrefix(line, "<![CDATA["):
		return "]]>", true
	case len(line) > 2 && strings.HasPrefix(line, "<!") && line[2] >= 'A' && line[2] <= 'Z':
		return ">", true
	}
	tag := htmlTag.FindStringSubmatch(line)
	if tag == nil {
		return "", false
	}
	if !strings.HasPrefix(line, "</") {
		switch strings.ToLower(tag[1]) {
		case "script", "pre", "style", "textarea":
			return "</" + strings.ToLower(tag[1]) + ">", true
		}
	}
	return "", true
}
