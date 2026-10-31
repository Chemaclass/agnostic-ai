package cli

import (
	"fmt"
	"slices"
	"strings"
)

const grokTOMLFormat = "grok-toml"
const grokModelsHeaderNewline = "header-newline"
const grokEmptyTableBody = "empty-body"
const grokModelsContainer = "\x00grok-models-separator"

func editGrokTOML(path string, data []byte, order []string, set map[string]any, remove []string, previous map[string]any) ([]byte, any, error) {
	for _, key := range append(append([]string{}, order...), remove...) {
		member, ui := strings.CutPrefix(key, "ui.")
		if key != "models.default" && key != "ui" && (!ui || member == "" || strings.Contains(member, ".")) {
			return nil, nil, fmt.Errorf("%s: global Grok settings support models.default and direct ui keys only", path)
		}
	}
	containers := grokTableSeparators(previous[grokModelsContainer])
	for _, table := range []string{"models", "ui"} {
		local := map[string]any{}
		var localOrder, localRemove []string
		for _, key := range order {
			if member, ok := strings.CutPrefix(key, table+"."); ok {
				local[member] = set[key]
				localOrder = append(localOrder, member)
			}
		}
		for _, key := range remove {
			if member, ok := strings.CutPrefix(key, table+"."); ok {
				localRemove = append(localRemove, member)
			}
		}
		parent, replace := set[table]
		if replace {
			value, ok := parent.(map[string]any)
			if !ok || len(value) != 0 {
				return nil, nil, fmt.Errorf("%s: %s must be an empty table or individual native keys", path, table)
			}
		}
		drop := slices.Contains(remove, table)
		if len(localOrder) == 0 && len(localRemove) == 0 && !replace && !drop {
			continue
		}
		var err error
		data, err = editGrokTable(path, data, table, localOrder, local, localRemove, replace, drop, containers)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(containers) == 0 {
		return data, nil, nil
	}
	return data, containers, nil
}

func grokTableSeparators(value any) map[string]string {
	separators := map[string]string{}
	switch stored := value.(type) {
	case string:
		separators["models"] = stored
	case map[string]string:
		for table, separator := range stored {
			separators[table] = separator
		}
	case map[string]any:
		for table, value := range stored {
			if separator, ok := value.(string); ok {
				separators[table] = separator
			}
		}
	}
	return separators
}

func grokOwnsTable(owned map[string]any) bool {
	for key := range owned {
		if key == "models.default" || key == "ui" || strings.HasPrefix(key, "ui.") {
			return true
		}
	}
	return false
}

func editGrokTable(path string, data []byte, table string, order []string, set map[string]any, remove []string, replace, drop bool, containers map[string]string) ([]byte, error) {
	if replace || drop {
		sections, err := grokTableSections(data, table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(sections) > 0 && (replace || len(sections) > 1) {
			for i := len(sections) - 1; i >= 0; i-- {
				section := sections[i]
				replacement := ""
				if replace && i == 0 {
					replacement = "[" + table + "]\n"
				}
				data = append(append(append([]byte(nil), data[:section.start]...), replacement...), data[section.end:]...)
			}
			if drop {
				delete(containers, table)
			}
			return data, nil
		}
	}
	lines := strings.SplitAfter(string(data), "\n")
	scan := make([]string, len(lines))
	for i, line := range lines {
		scan[i] = strings.TrimSuffix(line, "\n")
	}
	headers, err := scanTOMLHeaders(scan)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	start, end := -1, len(lines)
	nextTable := ""
	for i, h := range headers {
		if len(h.key) == 1 && h.key[0] == table {
			start = h.line
			for _, next := range headers[i+1:] {
				if (replace || drop) && len(next.key) > 0 && next.key[0] == table {
					continue
				}
				end = next.line
				if len(next.key) == 1 {
					nextTable = next.key[0]
				}
				break
			}
			break
		}
	}
	sep, owned := containers[table]
	if start >= 0 {
		body := strings.Join(lines[start+1:end], "")
		following := ""
		if separator, ok := containers[nextTable]; ok && separator != grokModelsHeaderNewline && separator != grokEmptyTableBody && strings.HasSuffix(body, separator) {
			body = strings.TrimSuffix(body, separator)
			following = separator
		}
		updated, err := editTOMLRoot(path, []byte(body), order, set, remove)
		if err != nil {
			return nil, err
		}
		if replace || drop {
			updated = nil
		}
		if !replace && sep == grokEmptyTableBody && len(remove) > 0 && strings.TrimSpace(string(updated)) == "" {
			updated = nil
			delete(containers, table)
		}
		if !replace && sep == grokModelsHeaderNewline && (len(remove) > 0 || drop) && strings.TrimSpace(string(updated)) == "" {
			prefix := strings.Join(lines[:start+1], "")
			prefix = strings.TrimSuffix(prefix, "\n")
			delete(containers, table)
			return []byte(prefix + following + strings.Join(lines[end:], "")), nil
		}
		if drop || !replace && owned && sep != grokModelsHeaderNewline && sep != grokEmptyTableBody && len(remove) > 0 && strings.TrimSpace(string(updated)) == "" {
			prefix := strings.Join(lines[:start], "")
			if owned {
				prefix = strings.TrimSuffix(prefix, sep)
			}
			delete(containers, table)
			return []byte(prefix + following + strings.Join(lines[end:], "")), nil
		}
		prefix := strings.Join(lines[:start+1], "")
		if (len(set) > 0 || replace) && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
			containers[table] = grokModelsHeaderNewline
		} else if len(set) > 0 && body == "" && !owned {
			containers[table] = grokEmptyTableBody
		}
		return []byte(prefix + string(updated) + following + strings.Join(lines[end:], "")), nil
	}
	if len(set) == 0 && !replace {
		delete(containers, table)
		return data, nil
	}
	values, err := settingsValues(path, "toml", data)
	if err != nil {
		return nil, err
	}
	if _, ok := values[table]; ok {
		return nil, fmt.Errorf("%s: %s uses a dotted or inline table; rewrite it as [%s] before syncing its settings", path, table, table)
	}
	body, err := editTOMLRoot(path, []byte{}, order, set, nil)
	if err != nil {
		return nil, err
	}
	if replace {
		body = nil
	}
	sep = ""
	if len(data) > 0 {
		sep = "\n"
		if !strings.HasSuffix(string(data), "\n") {
			sep = "\n\n"
		}
	}
	containers[table] = sep
	return []byte(string(data) + sep + "[" + table + "]\n" + string(body)), nil
}

type grokTableSection struct {
	start, end int
	before     []string
	array      bool
	occurrence int
}

func grokTableSections(data []byte, table string) ([]grokTableSection, error) {
	lines := strings.SplitAfter(string(data), "\n")
	scan := make([]string, len(lines))
	offsets := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		scan[i] = strings.TrimSuffix(line, "\n")
		offsets[i] = offset
		offset += len(line)
	}
	scan[0] = strings.TrimPrefix(scan[0], "\xef\xbb\xbf")
	headers, err := scanTOMLHeaders(scan)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	occurrences := make([]int, len(headers))
	for i, header := range headers {
		identity := fmt.Sprintf("%t:%s", header.array, strings.Join(header.key, "\x00"))
		counts[identity]++
		occurrences[i] = counts[identity]
	}
	var sections []grokTableSection
	for i := 0; i < len(headers); {
		header := headers[i]
		if len(header.key) == 0 || header.key[0] != table {
			i++
			continue
		}
		section := grokTableSection{start: offsets[header.line], end: len(data)}
		if header.line == 0 && strings.HasPrefix(string(data), "\xef\xbb\xbf") {
			section.start += len("\xef\xbb\xbf")
		}
		j := i + 1
		for j < len(headers) && len(headers[j].key) > 0 && headers[j].key[0] == table {
			j++
		}
		if j < len(headers) {
			next := headers[j]
			section.end = offsets[next.line]
			section.before = slices.Clone(next.key)
			section.array = next.array
			section.occurrence = occurrences[j]
		}
		sections = append(sections, section)
		i = j
	}
	return sections, nil
}
