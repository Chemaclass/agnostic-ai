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
	if format != "toml" && format != grokTOMLFormat {
		return 0, 0, false
	}
	text, bom := strings.CutPrefix(string(data), "\xef\xbb\xbf")
	lines := strings.SplitAfter(text, "\n")
	scan := make([]string, len(lines))
	for i, line := range lines {
		scan[i] = strings.TrimSuffix(line, "\n")
	}
	start, end := 0, len(lines)
	if format == grokTOMLFormat {
		table, member, nested := strings.Cut(key, ".")
		if !nested {
			return 0, 0, false
		}
		headers, err := scanTOMLHeaders(scan)
		if err != nil {
			return 0, 0, false
		}
		found := false
		for i, header := range headers {
			if len(header.key) != 1 || header.key[0] != table {
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
