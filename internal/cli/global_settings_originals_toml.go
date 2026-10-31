package cli

import (
	"slices"
	"strings"
)

type tomlPersonalTable struct {
	Text       string   `json:"text"`
	Before     []string `json:"before,omitempty"`
	Array      bool     `json:"array,omitempty"`
	Occurrence int      `json:"occurrence,omitempty"`
}

func restoreTOMLTables(data []byte, table string, originals []tomlPersonalTable) ([]byte, error) {
	sections, err := grokTableSections(data, table)
	if err != nil {
		return nil, err
	}
	if len(sections) > 0 && len(originals) > 0 {
		current := string(data[sections[0].start:sections[0].end])
		header, body, hasNewline := strings.Cut(current, "\n")
		if hasNewline {
			header += "\n"
		}
		if header != "["+table+"]\n" || body != "" {
			originals = slices.Clone(originals)
			merged := false
			for i, original := range originals {
				lines := strings.SplitAfter(original.Text, "\n")
				scan := make([]string, len(lines))
				for j, line := range lines {
					scan[j] = strings.TrimSuffix(line, "\n")
				}
				headers, err := scanTOMLHeaders(scan)
				if err != nil {
					return nil, err
				}
				for _, h := range headers {
					if h.array || !slices.Equal(h.key, []string{table}) {
						continue
					}
					originalHeader := lines[h.line]
					if header == "["+table+"]\n" {
						header = originalHeader
					}
					ending := "\n"
					if strings.HasSuffix(originalHeader, "\r\n") {
						ending = "\r\n"
					}
					if !strings.HasSuffix(header, "\n") {
						header += ending
					}
					if body != "" && !strings.HasSuffix(body, "\n") {
						body += ending
					}
					lines[h.line] = header + body
					originals[i].Text = strings.Join(lines, "")
					merged = true
					break
				}
				if merged {
					break
				}
			}
			if !merged {
				if !strings.HasSuffix(current, "\n") {
					current += "\n"
				}
				originals[0].Text = current + originals[0].Text
			}
		}
	}
	for i := len(sections) - 1; i >= 0; i-- {
		section := sections[i]
		data = append(append([]byte(nil), data[:section.start]...), data[section.end:]...)
	}
	for _, original := range originals {
		position := len(data)
		if len(original.Before) > 0 {
			lines := strings.SplitAfter(string(data), "\n")
			scan := make([]string, len(lines))
			for i, line := range lines {
				scan[i] = strings.TrimSuffix(line, "\n")
			}
			headers, err := scanTOMLHeaders(scan)
			if err != nil {
				return nil, err
			}
			occurrence := 0
			for _, header := range headers {
				if header.array != original.Array || !slices.Equal(header.key, original.Before) {
					continue
				}
				occurrence++
				if occurrence == original.Occurrence {
					position = len(strings.Join(lines[:header.line], ""))
					break
				}
			}
		}
		text := original.Text
		if position > 0 && data[position-1] != '\n' {
			text = "\n" + text
		}
		if position < len(data) && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		data = append(append(append([]byte(nil), data[:position]...), text...), data[position:]...)
	}
	return data, nil
}

func tomlPersonalSpan(data []byte, key, format string) (int, int, bool) {
	if format != "toml" && !nestedTOMLFormat(format) {
		return 0, 0, false
	}
	text, bom := strings.CutPrefix(string(data), "\xef\xbb\xbf")
	lines := strings.SplitAfter(text, "\n")
	scan := make([]string, len(lines))
	for i, line := range lines {
		scan[i] = strings.TrimSuffix(line, "\n")
	}
	start, end := 0, len(lines)
	if nestedTOMLFormat(format) {
		at := strings.LastIndexByte(key, '.')
		if at < 0 {
			return 0, 0, false
		}
		table, member := key[:at], key[at+1:]
		headers, err := scanTOMLHeaders(scan)
		if err != nil {
			return 0, 0, false
		}
		found := false
		for i, header := range headers {
			if !slices.Equal(header.key, strings.Split(table, ".")) {
				continue
			}
			start, found = header.line+1, true
			if i+1 < len(headers) {
				end = headers[i+1].line
			}
			break
		}
		if !found {
			return 0, 0, false
		}
		key = member
	}
	keys, _, err := scanTOMLRoot(scan[start:end])
	if err != nil {
		return 0, 0, false
	}
	location, found := keys[key]
	if !found {
		return 0, 0, false
	}
	end = start + location.end + 1
	start += location.start
	offset := 0
	if bom {
		offset = len("\xef\xbb\xbf")
	}
	return offset + len(strings.Join(lines[:start], "")), offset + len(strings.Join(lines[:end], "")), true
}

func restoreTOMLAssignment(original, current, key string, previous any) string {
	if at := strings.LastIndexByte(key, '.'); at >= 0 {
		key = key[at+1:]
	}
	expected, err := editTOMLRoot("", []byte(original), []string{key}, map[string]any{key: previous}, nil)
	if current == original || err == nil && current == string(expected) {
		return original
	}
	originalStart, originalEnd, originalOK := tomlAssignmentValueSpan(original)
	currentStart, currentEnd, currentOK := tomlAssignmentValueSpan(current)
	if !originalOK || !currentOK {
		return current
	}
	return current[:currentStart] + original[originalStart:originalEnd] + current[currentEnd:]
}

func tomlAssignmentValueSpan(assignment string) (int, int, bool) {
	eq := strings.IndexByte(assignment, '=')
	if eq < 0 {
		return 0, 0, false
	}
	start := eq + 1
	for start < len(assignment) && (assignment[start] == ' ' || assignment[start] == '\t') {
		start++
	}
	end, depth := start, 0
	quote := ""
	for i := start; i < len(assignment); i++ {
		c := assignment[i]
		if quote != "" {
			if strings.HasPrefix(assignment[i:], quote) {
				i += len(quote) - 1
				if len(quote) == 3 {
					for n := 0; n < 2 && i+1 < len(assignment) && assignment[i+1] == quote[0]; n++ {
						i++
					}
				}
				quote = ""
			} else if c == '\\' && quote[0] == '"' {
				i++
			}
			end = i + 1
			continue
		}
		switch c {
		case '\'', '"':
			quote = assignment[i : i+1]
			if strings.HasPrefix(assignment[i:], strings.Repeat(quote, 3)) {
				quote = strings.Repeat(quote, 3)
				i += 2
			}
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case '#':
			if depth == 0 {
				return start, end, end > start
			}
			for i < len(assignment) && assignment[i] != '\n' {
				i++
			}
		case '\n', '\r':
			if depth == 0 {
				return start, end, end > start
			}
		}
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			end = i + 1
		}
	}
	return start, end, end > start
}
