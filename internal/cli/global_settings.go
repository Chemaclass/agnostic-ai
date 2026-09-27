package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalSettingsFile maps the portable settings fields onto the native
// keys of one target's user settings file. A JSON key is a dotted path
// into nested objects.
type globalSettingsFile struct {
	// path is the user settings file, in the globalTargets path form.
	path string
	// format is "json" or "toml".
	format string
	// model and effort are the native keys; an empty one has no mapping.
	model, effort string
	effortLevel   func() []string
	// reserved are keys an x-<target> block may not set: surfaces sync
	// owns another way, or ones that need their own design.
	reserved map[string]string
}

// accepts reports whether the effort key takes value. A nil level list
// accepts any string.
func (f globalSettingsFile) accepts(value any) bool {
	level, ok := value.(string)
	if !ok || level == "" || f.effort == "" {
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
	target, key, field, source string
	value                      any
}

// globalSettingsFor resolves the settings specs for target into the
// native keys its user settings file takes: model and effort, then each
// x-<target> key. A field the target cannot take, or has no mapping for,
// raises a coverage note.
func globalSettingsFor(target string, g globalTarget, settings []spec.Entry) []globalSetting {
	f := g.settings
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
	if f.path == "" {
		const reason = "sync --global does not write this target's user settings yet"
		adapters.NoteSettingsFieldNoOp(target, "model", models, reason)
		adapters.NoteSettingsFieldNoOp(target, "effort", efforts, reason)
		adapters.NoteSettingsFieldNoOp(target, "x-"+target, custom, reason)
		return nil
	}
	var out []globalSetting
	if model := adapters.SettingsModel(settings, target); model != "" {
		if f.model == "" {
			adapters.NoteSettingsFieldNoOp(target, "model", models, "its user settings file has no default model key")
		} else {
			out = append(out, globalSetting{target: target, key: f.model, value: model, field: "model", source: lastSettingsSource(settings, "model", target)})
		}
	}
	if effort := adapters.SettingsEffort(settings, target); effort != nil {
		switch {
		case f.effort == "":
			adapters.NoteSettingsFieldNoOp(target, "effort", efforts, "its user settings file has no default effort key")
		case f.accepts(effort):
			out = append(out, globalSetting{target: target, key: f.effort, value: effort, field: "effort", source: lastSettingsSource(settings, "effort", target)})
		default:
			adapters.NoteSettingsFieldNoOp(target, "effort", 1, fmt.Sprintf("%s does not accept %v; it takes %s", f.effort, effort, f.levelsText()))
		}
	}
	passthrough := customGlobalSettings(target, f, settings)
	// An x-<target> key wins over the portable field it shares a key with.
	out = slices.DeleteFunc(out, func(s globalSetting) bool {
		return slices.ContainsFunc(passthrough, func(c globalSetting) bool { return c.key == s.key })
	})
	return append(out, passthrough...)
}

// customGlobalSettings flattens each spec's x-<target> block into native
// keys, later specs winning key by key and a null removing a key an
// earlier spec set. A JSON object merges field by field, as project
// sync merges x-<target> blocks; TOML takes scalars and arrays only.
func customGlobalSettings(target string, f globalSettingsFile, settings []spec.Entry) []globalSetting {
	byKey := map[string]globalSetting{}
	for _, entry := range settings {
		block, _ := entry.Meta["x-"+target].(map[string]any)
		for key, value := range flattenSettings(block, f.format) {
			if reason, ok := f.reserved[strings.SplitN(key, ".", 2)[0]]; ok {
				adapters.NoteSettingsFieldNoOp(target, "x-"+target+"."+key, 1, reason)
				continue
			}
			if value == nil {
				delete(byKey, key)
				continue
			}
			if f.format == "toml" {
				if _, err := tomlValueText(value); err != nil {
					adapters.NoteSettingsFieldNoOp(target, "x-"+target+"."+key, 1, "sync --global writes top-level scalars and arrays of config.toml, not tables")
					continue
				}
			}
			byKey[key] = globalSetting{target: target, key: key, value: value, field: "x-" + target + "." + key, source: entry.Path}
		}
	}
	out := make([]globalSetting, 0, len(byKey))
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[key])
	}
	return out
}

// flattenSettings turns nested JSON objects into dotted keys, so each
// leaf is owned on its own and the user's sibling keys stay. TOML keys
// stay whole.
func flattenSettings(block map[string]any, format string) map[string]any {
	out := map[string]any{}
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for key, value := range m {
			path := prefix + key
			if nested, ok := value.(map[string]any); ok && format == "json" && len(nested) > 0 {
				walk(path+".", nested)
				continue
			}
			out[path] = value
		}
	}
	walk("", block)
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
	owned map[string]any
}

// mergeGlobalSettings sets the managed keys in a user settings file and
// removes the ones an earlier sync wrote that the specs no longer set,
// leaving every other key and line as it is. base is the content to
// start from; nil reads path.
func mergeGlobalSettings(path, format string, base []byte, want []globalSetting, previous map[string]any) (globalSettingsMerge, error) {
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
	m := globalSettingsMerge{owned: map[string]any{}}
	set := map[string]any{}
	var remove []string
	wanted := map[string]bool{}
	for _, s := range want {
		wanted[s.key] = true
		have, present := settingsLookup(current, s.key, format)
		recorded, owned := previous[s.key]
		switch {
		case !present:
			m.changes = append(m.changes, fmt.Sprintf("set %s = %s", s.key, settingsValueText(s.value)))
			set[s.key] = s.value
		case sameSetting(have, s.value):
			if !owned || !sameSetting(recorded, s.value) {
				m.adopted = append(m.adopted, s.key)
			}
		case owned && sameSetting(have, recorded):
			m.changes = append(m.changes, fmt.Sprintf("set %s = %s", s.key, settingsValueText(s.value)))
			set[s.key] = s.value
		default:
			m.changes = append(m.changes, fmt.Sprintf("overwrite %s = %s with %s", s.key, settingsValueText(have), settingsValueText(s.value)))
			m.conflicts = append(m.conflicts, fmt.Sprintf("%s is %s, set outside agnostic-ai, and sync writes %s; to keep it, %s in %s",
				s.key, settingsValueText(have), settingsValueText(s.value), settingsKeepHint(s, have), s.source))
			set[s.key] = s.value
		}
		m.owned[s.key] = s.value
	}
	for _, key := range slices.Sorted(maps.Keys(previous)) {
		if wanted[key] {
			continue
		}
		// A value changed since sync wrote it is the user's now.
		if have, ok := settingsLookup(current, key, format); ok && sameSetting(have, previous[key]) {
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

// settingsLookup reads key from decoded settings: a dotted path into
// nested objects for JSON, the key itself for TOML.
func settingsLookup(values map[string]any, key, format string) (any, bool) {
	if format == "toml" {
		v, ok := values[key]
		return v, ok
	}
	var current any = values
	for _, part := range strings.Split(key, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = object[part]; !ok {
			return nil, false
		}
	}
	return current, true
}

// sameSetting compares two values as JSON sees them, so a TOML int64,
// a YAML int, and a recorded float64 of one number are equal.
func sameSetting(a, b any) bool {
	return reflect.DeepEqual(jsonRoundTrip(a), jsonRoundTrip(b))
}

// settingsKeepHint is the spec edit that keeps have as the value.
func settingsKeepHint(s globalSetting, have any) string {
	if strings.HasPrefix(s.field, "x-") {
		return fmt.Sprintf("set %s to %s", s.field, settingsValueText(have))
	}
	return fmt.Sprintf("put %s: %s under %s", s.target, settingsValueText(have), s.field)
}

func settingsValueText(v any) string {
	return jsonValueText(v, "", "")
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
