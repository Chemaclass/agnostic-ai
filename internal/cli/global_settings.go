package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalSettingsFile maps the portable settings fields onto the native
// keys of one target's user settings file.
type globalSettingsFile struct {
	// path is the user settings file, in the globalTargets path form.
	path string
	// format is "json" or "toml".
	format      string
	model       string
	effort      string
	effortLevel func() []string
}

// accepts reports whether the effort key takes value. A nil level list
// accepts any string.
func (f globalSettingsFile) accepts(value any) bool {
	level, ok := value.(string)
	if !ok || level == "" {
		return false
	}
	levels := f.effortLevel()
	return levels == nil || slices.Contains(levels, level)
}

func (f globalSettingsFile) levelsText() string {
	if levels := f.effortLevel(); levels != nil {
		return strings.Join(levels, ", ")
	}
	return "a string"
}

// globalSetting is one native key sync writes, with the spec field and
// file it comes from, for messages.
type globalSetting struct {
	target, key, value, field, source string
}

// globalSettingsFor resolves the settings specs for target into the
// native keys its user settings file takes, in a fixed order. A field
// the target cannot take, or has no mapping for, raises a coverage note.
func globalSettingsFor(target string, g globalTarget, settings []spec.Entry) []globalSetting {
	models, efforts, permissions, custom := 0, 0, 0, 0
	for _, entry := range settings {
		one := []spec.Entry{entry}
		if adapters.SettingsModel(one, target) != "" {
			models++
		}
		if adapters.SettingsEffort(one, target) != nil {
			efforts++
		}
		if _, ok := entry.Meta["permissions"]; ok {
			permissions++
		}
		if _, ok := entry.Meta["x-"+target]; ok {
			custom++
		}
	}
	adapters.NoteSettingsFieldNoOp(target, "permissions", permissions, "sync --global does not write user-level permissions")
	adapters.NoteSettingsFieldNoOp(target, "x-"+target, custom, "sync --global does not write target-specific settings")
	f := g.settings
	if f.path == "" {
		adapters.NoteSettingsFieldNoOp(target, "model", models, "sync --global does not write this target's user settings yet")
		adapters.NoteSettingsFieldNoOp(target, "effort", efforts, "sync --global does not write this target's user settings yet")
		return nil
	}
	var out []globalSetting
	if model := adapters.SettingsModel(settings, target); model != "" {
		out = append(out, globalSetting{target: target, key: f.model, value: model, field: "model", source: lastSettingsSource(settings, "model", target)})
	}
	if effort := adapters.SettingsEffort(settings, target); effort != nil {
		if f.accepts(effort) {
			out = append(out, globalSetting{target: target, key: f.effort, value: effort.(string), field: "effort", source: lastSettingsSource(settings, "effort", target)})
		} else {
			adapters.NoteSettingsFieldNoOp(target, "effort", 1, fmt.Sprintf("%s does not accept %v; it takes %s", f.effort, effort, f.levelsText()))
		}
	}
	return out
}

// lastSettingsSource names the spec file the winning value of field
// comes from.
func lastSettingsSource(settings []spec.Entry, field, target string) string {
	var source string
	for _, entry := range settings {
		one := []spec.Entry{entry}
		if (field == "model" && adapters.SettingsModel(one, target) != "") ||
			(field == "effort" && adapters.SettingsEffort(one, target) != nil) {
			source = entry.Path
		}
	}
	return source
}

// globalSettingsMerge is the planned change to one user settings file.
type globalSettingsMerge struct {
	// data is the new content, nil when the file needs no change.
	data []byte
	// changes lists each key the write sets or removes.
	changes []string
	// adopted lists keys that already held the value sync writes.
	adopted []string
	// conflicts lists keys set outside agnostic-ai to another value;
	// data overwrites them, which only --backup allows.
	conflicts []string
	// owned is the value per key sync owns after the write.
	owned map[string]string
}

// mergeGlobalSettings sets the managed keys in a user settings file and
// removes the ones an earlier sync wrote that the specs no longer set,
// leaving every other key and line as it is. base is the content to
// start from; nil reads path.
func mergeGlobalSettings(path, format string, base []byte, want []globalSetting, previous map[string]string) (globalSettingsMerge, error) {
	data := base
	absent := false
	if data == nil {
		read, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			absent = true
		case err != nil:
			return globalSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
	}
	if absent && len(want) == 0 {
		return globalSettingsMerge{}, nil
	}
	current, err := settingsValues(path, format, data)
	if err != nil {
		return globalSettingsMerge{}, err
	}
	m := globalSettingsMerge{owned: map[string]string{}}
	set := map[string]string{}
	var remove []string
	wanted := map[string]bool{}
	for _, s := range want {
		wanted[s.key] = true
		have, present := current[s.key]
		switch {
		case !present:
			m.changes = append(m.changes, fmt.Sprintf("set %s = %q", s.key, s.value))
			set[s.key] = s.value
		case have == s.value:
			if previous[s.key] != s.value {
				m.adopted = append(m.adopted, s.key)
			}
		case previous[s.key] != "" && have == previous[s.key]:
			m.changes = append(m.changes, fmt.Sprintf("set %s = %q", s.key, s.value))
			set[s.key] = s.value
		default:
			m.changes = append(m.changes, fmt.Sprintf("overwrite %s = %s with %q", s.key, settingsValueText(have), s.value))
			m.conflicts = append(m.conflicts, fmt.Sprintf("%s is %s, set outside agnostic-ai, and sync writes %q; to keep it, put %s under %s in %s",
				s.key, settingsValueText(have), s.value, settingsSpecLine(s, have), s.field, s.source))
			set[s.key] = s.value
		}
		m.owned[s.key] = s.value
	}
	for _, key := range slices.Sorted(maps.Keys(previous)) {
		if wanted[key] {
			continue
		}
		// A value changed since sync wrote it is the user's now.
		if have, ok := current[key]; ok && have == previous[key] {
			m.changes = append(m.changes, "remove "+key)
			remove = append(remove, key)
		}
	}
	if len(set) == 0 && len(remove) == 0 {
		return m, nil
	}
	order := make([]string, 0, len(set))
	for _, s := range want {
		if _, ok := set[s.key]; ok {
			order = append(order, s.key)
		}
	}
	if format == "toml" {
		m.data, err = editTOMLRoot(path, data, order, set, remove)
		return m, err
	}
	m.data, err = editJSONRoot(path, data, order, set, remove)
	return m, err
}

// settingsSpecLine is the target entry that keeps have as the value.
func settingsSpecLine(s globalSetting, have any) string {
	return fmt.Sprintf("%s: %q", s.target, fmt.Sprint(have))
}

func settingsValueText(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// settingsValues decodes the top-level keys of a user settings file.
func settingsValues(path, format string, data []byte) (map[string]any, error) {
	out := map[string]any{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, nil
	}
	if format == "toml" {
		if _, err := toml.Decode(string(data), &out); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return out, nil
	}
	stripped, _ := adapters.StripJSONC(data)
	if err := json.Unmarshal(stripped, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

// editJSONRoot sets and removes top-level keys in the text itself, so
// formatting, comments, and every other key stay byte for byte. A new
// key goes after the last member; removing it undoes that exactly.
func editJSONRoot(path string, data []byte, order []string, set map[string]string, remove []string) ([]byte, error) {
	text := strings.TrimRight(string(data), " \t\r\n")
	trailing := string(data)[len(text):]
	if text == "" {
		text, trailing = "{}", "\n"
	}
	edit := func(apply func(members []jsonMember, open, closing int) string) error {
		members, open, closing, err := scanJSONRoot(text)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		text = apply(members, open, closing)
		return nil
	}
	for _, key := range remove {
		err := edit(func(members []jsonMember, open, closing int) string {
			i := lastJSONMember(members, key)
			switch {
			case i < 0:
				return text
			case i > 0:
				m := members[i]
				return text[:m.comma] + text[m.comma+1:keptBefore(text, m.comma+1, m.start)] + text[m.end:]
			case len(members) > 1:
				return text[:members[0].start] + text[members[1].start:]
			default:
				m := members[0]
				if strings.TrimSpace(text[open+1:m.start]) == "" {
					return text[:open+1] + text[closing:]
				}
				return text[:keptBefore(text, open+1, m.start)] + text[m.end:]
			}
		})
		if err != nil {
			return nil, err
		}
	}
	for _, key := range order {
		value := jsonString(set[key])
		err := edit(func(members []jsonMember, open, closing int) string {
			// JSON decoding keeps the last of duplicate keys, so edit that one.
			if i := lastJSONMember(members, key); i >= 0 {
				return text[:members[i].valueStart] + value + text[members[i].end:]
			}
			member := jsonString(key) + ": " + value
			if len(members) == 0 {
				return text[:open+1] + "\n" + adapters.DetectJSONIndent(data) + member + "\n" + text[closing:]
			}
			first := members[0].start
			indent := text[strings.LastIndex(text[:first], "\n")+1 : first]
			if strings.TrimSpace(indent) != "" {
				indent = " "
				return text[:members[len(members)-1].end] + "," + indent + member + text[members[len(members)-1].end:]
			}
			last := members[len(members)-1].end
			return text[:last] + ",\n" + indent + member + text[last:]
		})
		if err != nil {
			return nil, err
		}
	}
	return []byte(text + trailing), nil
}

// keptBefore returns where the cut for a removed member starts: at the
// member itself when only whitespace precedes it from from, which
// undoes an insert exactly, else after the last line break before it,
// so a comment there stays on its own line.
func keptBefore(text string, from, start int) int {
	between := text[from:start]
	if strings.TrimSpace(between) == "" {
		return from
	}
	if nl := strings.LastIndex(between, "\n"); nl >= 0 {
		return from + nl + 1
	}
	return start
}

// jsonMember is one top-level member of a JSON object: the comma
// before it (-1 for the first), where its key starts, where its value
// starts, and where the value ends.
type jsonMember struct {
	key                           string
	comma, start, valueStart, end int
}

func lastJSONMember(members []jsonMember, key string) int {
	for i := len(members) - 1; i >= 0; i-- {
		if members[i].key == key {
			return i
		}
	}
	return -1
}

// scanJSONRoot lists the members of the top-level object in text and
// the offsets of its opening and closing braces. It skips strings and
// JSONC comments.
func scanJSONRoot(text string) ([]jsonMember, int, int, error) {
	var members []jsonMember
	depth := 0
	var current *jsonMember
	expectKey := false
	open, comma := -1, -1
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case strings.HasPrefix(text[i:], "//"):
			if nl := strings.IndexByte(text[i:], '\n'); nl >= 0 {
				i += nl
			} else {
				i = len(text)
			}
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return nil, 0, 0, fmt.Errorf("unterminated comment")
			}
			i += end + 3
		case c == '"':
			j := i + 1
			for ; j < len(text) && text[j] != '"'; j++ {
				if text[j] == '\\' {
					j++
				}
			}
			if j >= len(text) {
				return nil, 0, 0, fmt.Errorf("unterminated string")
			}
			if depth == 1 && expectKey {
				var key string
				if err := json.Unmarshal([]byte(text[i:j+1]), &key); err != nil {
					return nil, 0, 0, err
				}
				members = append(members, jsonMember{key: key, comma: comma, start: i})
				current = &members[len(members)-1]
				expectKey = false
			} else if depth == 1 && current != nil {
				current.end = j + 1
			}
			i = j
		case c == ':' && depth == 1 && current != nil && current.valueStart == 0:
			j := i + 1
			for j < len(text) && strings.ContainsRune(" \t\r\n", rune(text[j])) {
				j++
			}
			current.valueStart = j
		case c == '{' || c == '[':
			depth++
			if depth == 1 {
				expectKey = c == '{'
				open = i
			}
		case c == '}' || c == ']':
			depth--
			if depth == 1 && current != nil {
				current.end = i + 1
			}
			if depth == 0 {
				return members, open, i, nil
			}
		case c == ',' && depth == 1:
			expectKey = true
			current = nil
			comma = i
		case depth == 1 && current != nil && current.valueStart != 0 && !strings.ContainsRune(" \t\r\n", rune(c)):
			current.end = i + 1
		}
	}
	return nil, 0, 0, fmt.Errorf("expected a JSON object")
}

// tomlRootKey is where one root-table key sits: its first and last line,
// and a trailing comment on a one-line value.
type tomlRootKey struct {
	start, end int
	comment    string
}

// editTOMLRoot sets and removes keys of the root table line by line, so
// comments, tables, and formatting elsewhere stay byte for byte. A set
// key replaces its line, keeping a trailing comment; a new key goes
// after the last root key, or at the top when there is none, which is
// always before the first table header.
func editTOMLRoot(path string, data []byte, order []string, set map[string]string, remove []string) ([]byte, error) {
	// Each line keeps its own "\r", so mixed line endings survive; new
	// lines follow the first line's ending. A BOM stays in front.
	text, bom := strings.CutPrefix(string(data), "\ufeff")
	cr := ""
	if first, _, _ := strings.Cut(text, "\n"); strings.HasSuffix(first, "\r") {
		cr = "\r"
	}
	trailing := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	keys, lastKey, err := scanTOMLRoot(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	line := func(key, value string) string {
		return key + ` = "` + adapters.EscapeTOMLBasic(value) + `"` + cr
	}
	replaced := map[int][]string{}
	dropped := map[int]bool{}
	var added []string
	for _, key := range order {
		loc, ok := keys[key]
		if !ok {
			added = append(added, line(key, set[key]))
			continue
		}
		start := lines[loc.start]
		indent := start[:len(start)-len(strings.TrimLeft(start, " \t"))]
		next := indent + key + ` = "` + adapters.EscapeTOMLBasic(set[key]) + `"`
		if loc.comment != "" {
			next += " " + loc.comment
		}
		if strings.HasSuffix(start, "\r") {
			next += "\r"
		}
		replaced[loc.start] = []string{next}
		for i := loc.start + 1; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	for _, key := range remove {
		loc, ok := keys[key]
		if !ok {
			continue
		}
		for i := loc.start; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	insertAt := 0
	if lastKey >= 0 {
		insertAt = lastKey + 1
	} else if len(added) > 0 && len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		added = append(added, cr)
	}
	var out []string
	for i := 0; i <= len(lines); i++ {
		if i == insertAt {
			out = append(out, added...)
		}
		if i == len(lines) {
			break
		}
		if next, ok := replaced[i]; ok {
			out = append(out, next...)
			continue
		}
		if !dropped[i] {
			out = append(out, lines[i])
		}
	}
	result := strings.Join(out, "\n")
	if trailing || len(lines) == 0 {
		result += "\n"
	}
	if bom {
		result = "\ufeff" + result
	}
	return []byte(result), nil
}

// scanTOMLRoot finds the keys of the root table, the lines before the
// first table header, and the last line of the last root key (-1 when
// there is none). It tracks strings, comments, and nested arrays and
// inline tables, so a bracket inside a value is not read as a header.
func scanTOMLRoot(lines []string) (map[string]tomlRootKey, int, error) {
	keys := map[string]tomlRootKey{}
	lastKey := -1
	var multi string // the open multi-line string delimiter, if any
	depth := 0
	current := ""
	for i, raw := range lines {
		rest := raw
		if multi == "" && depth == 0 {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, "[") {
				return keys, lastKey, nil
			}
			eq := strings.Index(raw, "=")
			if eq < 0 {
				return nil, -1, fmt.Errorf("line %d: expected a key and value", i+1)
			}
			current = strings.Trim(strings.TrimSpace(raw[:eq]), `"'`)
			keys[current] = tomlRootKey{start: i, end: i}
			rest = raw[eq+1:]
		}
		comment := ""
		for j := 0; j < len(rest); j++ {
			c := rest[j]
			if multi != "" {
				if strings.HasPrefix(rest[j:], multi) {
					j += len(multi) - 1
					multi = ""
				} else if c == '\\' && multi == `"""` {
					j++
				}
				continue
			}
			switch {
			case strings.HasPrefix(rest[j:], `"""`), strings.HasPrefix(rest[j:], `'''`):
				multi = rest[j : j+3]
				j += 2
			case c == '"':
				for j++; j < len(rest) && rest[j] != '"'; j++ {
					if rest[j] == '\\' {
						j++
					}
				}
			case c == '\'':
				for j++; j < len(rest) && rest[j] != '\''; j++ {
				}
			case c == '[', c == '{':
				depth++
			case c == ']', c == '}':
				depth--
			case c == '#':
				comment = strings.TrimSpace(rest[j:])
				j = len(rest)
			}
		}
		loc := keys[current]
		loc.end = i
		if loc.start == i && multi == "" && depth == 0 {
			loc.comment = comment
		}
		keys[current] = loc
		if multi == "" && depth == 0 {
			lastKey = i
		}
	}
	if multi != "" || depth != 0 {
		return nil, -1, fmt.Errorf("unterminated value in the root table")
	}
	return keys, lastKey, nil
}

// lintGlobalSettings reports each settings effort a target that sync
// --global writes settings for cannot take, so lint and validate catch
// what a sync would drop with a note.
func lintGlobalSettings(settings []spec.Entry, targets []string) []validationIssue {
	var out []validationIssue
	for _, entry := range settings {
		for _, target := range slices.Sorted(slices.Values(targets)) {
			f := globalTargets[target].settings
			if f.path == "" {
				continue
			}
			effort := adapters.SettingsEffort([]spec.Entry{entry}, target)
			if effort == nil || f.accepts(effort) {
				continue
			}
			out = append(out, validationIssue{
				Path:    entry.Path,
				Field:   "effort",
				Message: fmt.Sprintf("%s: %s does not accept effort %v; it takes %s", target, f.effort, effort, f.levelsText()),
			})
		}
	}
	return out
}

// lintGlobalSettingsFindings reports the same values as lint findings
// (LINT014, error).
func lintGlobalSettingsFindings(settings []spec.Entry, targets []string) []lintFinding {
	var out []lintFinding
	for _, issue := range lintGlobalSettings(settings, targets) {
		out = append(out, lintFinding{Code: "LINT014", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}
