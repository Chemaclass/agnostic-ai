package emit

import "strings"

type sectionHeading struct {
	line, end, level int
}

func nestSectionHeadings(body string) string {
	lines := strings.Split(body, "\n")
	var headings []sectionHeading
	minimum, paragraph := 4, -1
	var fence byte
	fenceLength := 0
	for i, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent > 3 || strings.HasPrefix(line[indent:], "\t") {
			paragraph = -1
			continue
		}
		line = line[indent:]
		if fence != 0 {
			run := len(line) - len(strings.TrimLeft(line, string(fence)))
			if run >= fenceLength && strings.TrimSpace(line[run:]) == "" {
				fence = 0
			}
			continue
		}
		if len(line) > 0 && (line[0] == '`' || line[0] == '~') {
			run := len(line) - len(strings.TrimLeft(line, line[:1]))
			if run >= 3 && (line[0] == '~' || !strings.Contains(line[run:], "`")) {
				fence, fenceLength, paragraph = line[0], run, -1
				continue
			}
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level > 0 && level <= 6 && (level == len(line) || line[level] == ' ' || line[level] == '\t') {
			headings = append(headings, sectionHeading{i, i, level})
			minimum = min(minimum, level)
			paragraph = -1
			continue
		}
		underline := strings.TrimRight(line, " \t")
		if paragraph >= 0 && len(underline) > 0 && (underline[0] == '=' || underline[0] == '-') && strings.Trim(underline, underline[:1]) == "" {
			level := 1
			if underline[0] == '-' {
				level = 2
			}
			headings = append(headings, sectionHeading{paragraph, i, level})
			minimum = min(minimum, level)
			paragraph = -1
			continue
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") || sectionThematicBreak(line) {
			paragraph = -1
		} else if paragraph < 0 {
			paragraph = i
		}
	}
	if minimum == 4 {
		return body
	}
	removed := map[int]bool{}
	for _, heading := range headings {
		line := lines[heading.line]
		indent := len(line) - len(strings.TrimLeft(line, " "))
		level := min(6, heading.level+4-minimum)
		if heading.line == heading.end {
			lines[heading.line] = line[:indent] + strings.Repeat("#", level) + line[indent+heading.level:]
			continue
		}
		text := make([]string, 0, heading.end-heading.line)
		for i := heading.line; i < heading.end; i++ {
			text = append(text, strings.TrimSpace(lines[i]))
		}
		ending := ""
		if strings.HasSuffix(lines[heading.end], "\r") {
			ending = "\r"
		}
		lines[heading.line] = line[:indent] + strings.Repeat("#", level) + " " + strings.Join(text, " ") + ending
		for i := heading.line + 1; i <= heading.end; i++ {
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

func sectionThematicBreak(line string) bool {
	line = strings.Join(strings.Fields(line), "")
	return len(line) >= 3 && strings.ContainsAny(line[:1], "-*_") && strings.Trim(line, line[:1]) == ""
}
