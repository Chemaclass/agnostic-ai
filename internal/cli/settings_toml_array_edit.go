package cli

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Only inserted separator bytes and a hash of their predecessor are recorded.
type tomlArrayLayout struct {
	Prefix string `json:"prefix,omitempty"`
	Before string `json:"before,omitempty"`
}
type tomlArraySpan struct{ start, end int }
type tomlArrayRecord struct {
	value map[string]any
	spans []tomlArraySpan
	index int
}

func decodeTOMLDocument(path string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if _, err := toml.Decode(strings.TrimPrefix(string(data), "\xef\xbb\xbf"), &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}
func scanTOMLArrayWithIdentity(path string, data []byte, parent string, want map[string]map[string]any, previous map[string]any, unnamed bool) (map[string]tomlArrayRecord, bool, error) {
	doc, err := decodeTOMLDocument(path, data)
	if err != nil {
		return nil, false, err
	}
	records, err := nativeTOMLRecords(path, parent, doc[parent])
	if err != nil {
		return nil, false, err
	}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	bom := len(data) - len(text)
	lines := strings.SplitAfter(text, "\n")
	offset := make([]int, len(lines)+1)
	offset[0] = bom
	for i, line := range lines {
		offset[i+1] = offset[i] + len(line)
	}
	headers, err := scanTOMLHeaders(lines)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	out := map[string]tomlArrayRecord{}
	index := -1
	names, err := tomlArrayIdentities(path, parent, records, want, previous, unnamed)
	if err != nil {
		return nil, false, err
	}
	for i, r := range records {
		out[names[i]] = tomlArrayRecord{value: r, index: i}
	}

	for h, header := range headers {
		if len(header.key) == 0 || header.key[0] != parent {
			continue
		}
		if len(header.key) == 1 {
			if !header.array {
				return nil, false, fmt.Errorf("%s: use [[%s]] for native entries", path, parent)
			}
			index++
			if index >= len(records) {
				return nil, false, fmt.Errorf("%s: ambiguous %s array representation", path, parent)
			}
		}
		if index < 0 {
			continue
		}
		next := len(lines)
		if h+1 < len(headers) {
			next = headers[h+1].line
		}
		end := next
		for end > header.line+1 {
			trim := strings.TrimSpace(lines[end-1])
			if trim != "" && !strings.HasPrefix(trim, "#") {
				break
			}
			end--
		}
		for end < next && strings.HasPrefix(strings.TrimSpace(lines[end]), "#") {
			end++
		}
		name := names[index]
		r := out[name]
		r.spans = append(r.spans, tomlArraySpan{offset[header.line], offset[end]})
		out[name] = r
	}
	return out, index+1 != len(records), nil
}
func encodeTOMLArrayRecord(parent string, value map[string]any) ([]byte, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(map[string]any{parent: []map[string]any{value}}); err != nil {
		return nil, fmt.Errorf("encode [[%s]]: %w", parent, err)
	}
	return b.Bytes(), nil
}
func mergeTOMLArray(path string, data []byte, parent string, want map[string]map[string]any, previous map[string]any, layouts map[string]tomlArrayLayout) (globalSettingsMerge, map[string]tomlArrayLayout, error) {
	return mergeTOMLArrayRecords(path, data, parent, want, previous, layouts, false)
}

// unnamed keeps external spec identities in state, never in the native records.
func mergeTOMLArrayRecords(path string, data []byte, parent string, want map[string]map[string]any, previous map[string]any, layouts map[string]tomlArrayLayout, unnamed bool) (globalSettingsMerge, map[string]tomlArrayLayout, error) {
	transitioned := maps.Clone(previous)
	if transitioned == nil {
		transitioned = map[string]any{}
	}
	scan := func(data []byte) (map[string]tomlArrayRecord, bool, error) {
		return scanTOMLArrayWithIdentity(path, data, parent, want, transitioned, unnamed)
	}
	original, err := decodeTOMLDocument(path, data)
	if err != nil {
		return globalSettingsMerge{}, nil, err
	}
	current, inline, err := scan(data)
	if err != nil {
		return globalSettingsMerge{}, nil, err
	}
	nextLayouts := maps.Clone(layouts)
	if nextLayouts == nil {
		nextLayouts = map[string]tomlArrayLayout{}
	}
	proven := map[string]bool{}
	for id, layout := range layouts {
		record, ok := current[id]
		if !ok || len(record.spans) == 0 {
			continue
		}
		at := record.spans[0].start
		prefixAt := at - len(layout.Prefix)
		if prefixAt >= 0 && string(data[prefixAt:at]) == layout.Prefix && tomlArrayAnchor(data[:prefixAt]) == layout.Before {
			proven[id] = true
		}
	}
	m := globalSettingsMerge{owned: map[string]any{}}
	var remove, replace, add []string
	for _, name := range slices.Sorted(maps.Keys(want)) {
		value := want[name]
		have, present := current[name]
		old, owned := previous[name]
		switch {
		case !present:
			add = append(add, name)
		case sameSetting(have.value, value):
			if !owned {
				m.adopted = append(m.adopted, parent+"."+name)
			}
		case owned && sameSetting(have.value, old):
			replace = append(replace, name)
		default:
			return m, nil, fmt.Errorf("%s: %s.%s differs from the managed value; import it or make the spec match before syncing", path, parent, name)
		}
		m.owned[name] = jsonRoundTrip(value)
	}
	for name, old := range previous {
		if _, ok := want[name]; ok {
			continue
		}
		if have, ok := current[name]; ok {
			if !sameSetting(have.value, old) {
				return m, nil, fmt.Errorf("%s: %s.%s was edited outside agnostic-ai; import or remove it before syncing", path, parent, name)
			}
			remove = append(remove, name)
		} else {
			delete(nextLayouts, name)
		}
	}
	if len(remove)+len(replace)+len(add) == 0 {
		return m, nextLayouts, nil
	}
	if inline {
		return m, nil, fmt.Errorf("%s: cannot safely edit inline %s; use [[%s]] tables or import the file into source first", path, parent, parent)
	}
	result := slices.Clone(data)
	// Reverse file order lets provenance hashes restore several appended entries.
	slices.SortFunc(remove, func(a, b string) int { return current[b].spans[0].start - current[a].spans[0].start })
	splice := func(name string, fragment []byte, removing bool) error {
		now, _, err := scan(result)
		if err != nil {
			return err
		}
		r := now[name]
		if len(r.spans) == 0 {
			return fmt.Errorf("%s: %s.%s has no editable table", path, parent, name)
		}
		at := r.spans[0].start
		end := r.spans[len(r.spans)-1].end
		if removing {
			if layout, ok := nextLayouts[name]; ok && len(layout.Prefix) <= at && proven[name] {
				prefixAt := at - len(layout.Prefix)
				if string(result[prefixAt:at]) == layout.Prefix && (end == len(result) || prefixAt == 0 || bytes.HasSuffix(result[:prefixAt], []byte("\n")) || bytes.HasPrefix(result[end:], []byte("\n")) || bytes.HasPrefix(result[end:], []byte("\r\n"))) {
					r.spans[0].start = prefixAt
				}
			}
			delete(nextLayouts, name)
		}
		for i := len(r.spans) - 1; i >= 0; i-- {
			span := r.spans[i]
			insert := []byte(nil)
			if i == 0 {
				insert = fragment
			}
			result = append(append(append([]byte(nil), result[:span.start]...), insert...), result[span.end:]...)
		}
		return nil
	}
	for _, name := range remove {
		if err := splice(name, nil, true); err != nil {
			return m, nil, err
		}
		delete(transitioned, name)
		m.changes = append(m.changes, "remove [["+parent+"]] "+name)
	}
	for _, name := range replace {
		fragment, err := encodeTOMLArrayRecord(parent, want[name])
		if err != nil {
			return m, nil, err
		}
		if bytes.Contains(result, []byte("\r\n")) {
			fragment = bytes.ReplaceAll(fragment, []byte("\n"), []byte("\r\n"))
		}
		if err := splice(name, fragment, false); err != nil {
			return m, nil, err
		}
		transitioned[name] = jsonRoundTrip(want[name])
		m.changes = append(m.changes, "replace [["+parent+"]] "+name)
	}
	for _, name := range add {
		fragment, err := encodeTOMLArrayRecord(parent, want[name])
		if err != nil {
			return m, nil, err
		}
		nl := "\n"
		if bytes.Contains(result, []byte("\r\n")) {
			nl = "\r\n"
			fragment = bytes.ReplaceAll(fragment, []byte("\n"), []byte(nl))
		}
		prefix := ""
		if len(bytes.TrimPrefix(result, []byte("\xef\xbb\xbf"))) > 0 {
			prefix = nl
			if !bytes.HasSuffix(result, []byte("\n")) {
				prefix = nl + nl
			}
		}
		nextLayouts[name] = tomlArrayLayout{Prefix: prefix, Before: tomlArrayAnchor(result)}
		result = append(result, []byte(prefix)...)
		result = append(result, fragment...)
		transitioned[name] = jsonRoundTrip(want[name])
		m.changes = append(m.changes, "add [["+parent+"]] "+name)
	}
	after, err := decodeTOMLDocument(path, result)
	if err != nil {
		return m, nil, err
	}
	beforeRecords, err := nativeTOMLRecords(path, parent, original[parent])
	if err != nil {
		return m, nil, err
	}
	expected := []map[string]any{}
	for i, r := range beforeRecords {
		name := ""
		for id, record := range current {
			if record.index == i {
				name = id
				break
			}
		}
		if slices.Contains(remove, name) {
			continue
		}
		if slices.Contains(replace, name) {
			r = want[name]
		}
		expected = append(expected, r)
	}
	for _, name := range add {
		expected = append(expected, want[name])
	}
	if len(expected) > 0 {
		original[parent] = expected
	} else {
		delete(original, parent)
	}
	if !sameSetting(original, after) {
		return m, nil, fmt.Errorf("%s: editing [[%s]] changed unrelated TOML values; move entries to separate native tables first", path, parent)
	}
	final, _, err := scan(result)
	if err != nil {
		return m, nil, err
	}
	for id, layout := range nextLayouts {
		if !proven[id] {
			continue
		}
		record, ok := final[id]
		if !ok || len(record.spans) == 0 {
			continue
		}
		at := record.spans[0].start
		prefixAt := at - len(layout.Prefix)
		if prefixAt >= 0 && string(result[prefixAt:at]) == layout.Prefix {
			layout.Before = tomlArrayAnchor(result[:prefixAt])
			nextLayouts[id] = layout
		}
	}
	m.data = result
	if m.data == nil {
		m.data = []byte{}
	}
	return m, nextLayouts, nil
}

func namedTOMLRecords(path string, records []map[string]any) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for _, r := range records {
		name, _ := r["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("%s: native record requires a name", path)
		}
		if _, ok := out[name]; ok {
			return nil, fmt.Errorf("%s: duplicate native record %q", path, name)
		}
		out[name] = r
	}
	return out, nil
}
func namedTOMLAnyRecords(path string, records []any) (map[string]any, error) {
	out := map[string]any{}
	for _, r := range records {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: invalid native record in ownership state", path)
		}
		name, _ := m["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("%s: native ownership record requires name", path)
		}
		out[name] = m
	}
	return out, nil
}

func tomlArrayAnchor(data []byte) string {
	text := strings.TrimRight(string(data), "\r\n")
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		text = text[i+1:]
	}
	return globalSum([]byte(text))
}

func tomlArrayIdentities(path, parent string, records []map[string]any, want map[string]map[string]any, previous map[string]any, unnamed bool) ([]string, error) {
	names := make([]string, len(records))
	used := map[string]bool{}
	if !unnamed {
		for i, r := range records {
			name, _ := r["name"].(string)
			if name == "" {
				return nil, fmt.Errorf("%s: %s entry %d requires a name", path, parent, i+1)
			}
			if used[name] {
				return nil, fmt.Errorf("%s: duplicate %s name %q", path, parent, name)
			}
			names[i] = name
			used[name] = true
		}
		return names, nil
	}
	for _, id := range slices.Sorted(maps.Keys(previous)) {
		var matches []int
		for i, r := range records {
			if names[i] == "" && sameSetting(r, previous[id]) {
				matches = append(matches, i)
			}
		}
		if len(matches) > 1 {
			if desired, present := want[id]; !present || !sameSetting(desired, previous[id]) {
				return nil, fmt.Errorf("%s: %s entry %s has duplicate managed matches; remove the ambiguity before changing it", path, parent, id)
			}
		}
		if len(matches) > 0 {
			names[matches[0]] = id
			used[id] = true
			continue
		}
		old, ok := previous[id].(map[string]any)
		if !ok {
			continue
		}
		for i, r := range records {
			if names[i] != "" {
				continue
			}
			a, b := maps.Clone(old), maps.Clone(r)
			delete(a, "timeout")
			delete(b, "timeout")
			if sameSetting(a, b) {
				return nil, fmt.Errorf("%s: %s entry %s was edited outside agnostic-ai; import it before syncing", path, parent, id)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if strings.HasPrefix(id, "\x00") {
			return nil, fmt.Errorf("%s: invalid external array identity", path)
		}
		if used[id] {
			continue
		}
		for i, r := range records {
			if names[i] == "" && sameSetting(r, want[id]) {
				names[i] = id
				used[id] = true
				break
			}
		}
	}
	for i := range names {
		if names[i] == "" {
			names[i] = fmt.Sprintf("\x00unmanaged:%d", i)
		}
	}
	return names, nil
}

func rebaseTOMLArrayLayouts(path, parent string, before, after []byte, layouts map[string]tomlArrayLayout, want map[string]map[string]any, previous map[string]any, unnamed bool) (map[string]tomlArrayLayout, error) {
	next := maps.Clone(layouts)
	if len(layouts) == 0 || bytes.Equal(before, after) {
		return next, nil
	}
	original, _, err := scanTOMLArrayWithIdentity(path, before, parent, want, previous, unnamed)
	if err != nil {
		return nil, err
	}
	planned, _, err := scanTOMLArrayWithIdentity(path, after, parent, want, previous, unnamed)
	if err != nil {
		return nil, err
	}
	for id, layout := range layouts {
		old, oldPresent := original[id]
		current, present := planned[id]
		if !oldPresent || !present || len(old.spans) == 0 || len(old.spans) != len(current.spans) {
			continue
		}
		unchanged := true
		for i, span := range old.spans {
			nextSpan := current.spans[i]
			if !bytes.Equal(before[span.start:span.end], after[nextSpan.start:nextSpan.end]) {
				unchanged = false
				break
			}
		}
		if !unchanged {
			continue
		}
		oldAt, at := old.spans[0].start, current.spans[0].start
		oldPrefixAt, prefixAt := oldAt-len(layout.Prefix), at-len(layout.Prefix)
		if oldPrefixAt < 0 || prefixAt < 0 || string(before[oldPrefixAt:oldAt]) != layout.Prefix || tomlArrayAnchor(before[:oldPrefixAt]) != layout.Before || string(after[prefixAt:at]) != layout.Prefix {
			continue
		}
		layout.Before = tomlArrayAnchor(after[:prefixAt])
		next[id] = layout
	}
	return next, nil
}
