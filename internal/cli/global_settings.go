package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

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
	edit := func(apply func(members []jsonMember, closing int) string) error {
		members, closing, err := scanJSONRoot(text)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		text = apply(members, closing)
		return nil
	}
	for _, key := range remove {
		err := edit(func(members []jsonMember, closing int) string {
			i := slices.IndexFunc(members, func(m jsonMember) bool { return m.key == key })
			switch {
			case i < 0:
				return text
			case i > 0:
				return text[:members[i-1].end] + text[members[i].end:]
			case len(members) > 1:
				return text[:members[0].start] + text[members[1].start:]
			default:
				open := strings.LastIndex(text[:members[0].start], "{")
				return text[:open+1] + text[closing:]
			}
		})
		if err != nil {
			return nil, err
		}
	}
	for _, key := range order {
		value := jsonString(set[key])
		err := edit(func(members []jsonMember, closing int) string {
			if i := slices.IndexFunc(members, func(m jsonMember) bool { return m.key == key }); i >= 0 {
				return text[:members[i].valueStart] + value + text[members[i].end:]
			}
			member := jsonString(key) + ": " + value
			if len(members) == 0 {
				open := strings.LastIndex(text[:closing], "{")
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

// jsonMember is one top-level member of a JSON object: where its key
// starts, where its value starts, and where the value ends.
type jsonMember struct {
	key                    string
	start, valueStart, end int
}

// scanJSONRoot lists the members of the top-level object in text and
// the offset of its closing brace. It skips strings and JSONC comments.
func scanJSONRoot(text string) ([]jsonMember, int, error) {
	var members []jsonMember
	depth := 0
	var current *jsonMember
	expectKey := false
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
				return nil, 0, fmt.Errorf("unterminated comment")
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
				return nil, 0, fmt.Errorf("unterminated string")
			}
			if depth == 1 && expectKey {
				var key string
				if err := json.Unmarshal([]byte(text[i:j+1]), &key); err != nil {
					return nil, 0, err
				}
				members = append(members, jsonMember{key: key, start: i})
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
			}
		case c == '}' || c == ']':
			depth--
			if depth == 1 && current != nil {
				current.end = i + 1
			}
			if depth == 0 {
				return members, i, nil
			}
		case c == ',' && depth == 1:
			expectKey = true
			current = nil
		case depth == 1 && current != nil && current.valueStart != 0 && !strings.ContainsRune(" \t\r\n", rune(c)):
			current.end = i + 1
		}
	}
	return nil, 0, fmt.Errorf("expected a JSON object")
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
	eol := "\n"
	text := string(data)
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
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
		return key + ` = "` + adapters.EscapeTOMLBasic(value) + `"`
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
		next := indent + line(key, set[key])
		if loc.comment != "" {
			next += " " + loc.comment
		}
		replaced[loc.start] = []string{next}
		for i := loc.start + 1; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	for _, key := range remove {
		loc := keys[key]
		for i := loc.start; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	insertAt := 0
	if lastKey >= 0 {
		insertAt = lastKey + 1
	} else if len(added) > 0 && len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		added = append(added, "")
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
	return []byte(strings.ReplaceAll(result, "\n", eol)), nil
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

// runExplainGlobal names the user settings file and key each target
// gets from one global settings spec. A key a later spec overrides is
// not this spec's to claim.
func runExplainGlobal(cmd *cobra.Command, input string, jsonOut bool) error {
	scope, err := loadCheckScope(true)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(input) {
		if _, statErr := os.Stat(input); statErr != nil {
			input = filepath.Join(scope.source, input)
		}
	}
	entry, err := findSpecEntry(input, scope.bundle)
	if err != nil {
		return err
	}
	if entry.Kind != spec.KindSettings {
		return fmt.Errorf("explain --global covers settings specs; %s is a %s spec, so run list --global for its layer", entry.Path, entry.Kind)
	}
	home, err := globalUserHome()
	if err != nil {
		return err
	}
	settings := scope.bundle.Settings
	contributions := []contribution{}
	for _, target := range slices.Sorted(slices.Values(scope.targets)) {
		g := globalTargets[target]
		if g.settings.path == "" {
			continue
		}
		adapters.SetWarner(io.Discard)
		want := globalSettingsFor(target, g, settings)
		adapters.ResetCoverageNotes()
		adapters.SetWarner(os.Stderr)
		for _, s := range want {
			if s.source == entry.Path {
				contributions = append(contributions, contribution{Target: target, Path: g.path(home, g.settings.path), Section: s.key, Mode: "key"})
			}
		}
	}
	if jsonOut {
		return emitExplainJSON(cmd, explainOutput{
			Version:            "1",
			Command:            "explain",
			Spec:               explainSpecRef{Kind: string(entry.Kind), Name: entry.Name, Path: filepath.ToSlash(entry.Path)},
			Contributions:      contributions,
			WouldEmitIfEnabled: []contribution{},
		})
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "%s →\n", filepath.ToSlash(entry.Path)); err != nil {
		return fmt.Errorf("write explain output: %w", err)
	}
	if len(contributions) == 0 {
		_, err := fmt.Fprintln(out, "  (no target sync --global writes takes a key from this spec)")
		return err
	}
	for _, c := range contributions {
		if _, err := fmt.Fprintf(out, "  %s\n", formatContribution(c)); err != nil {
			return fmt.Errorf("write explain output: %w", err)
		}
	}
	return nil
}
