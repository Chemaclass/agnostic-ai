package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalSettingOriginal is what a native key held before a personal
// portable field first wrote it, so removal puts the same bytes back.
type globalSettingOriginal struct {
	Raw          string              `json:"raw,omitempty"`
	Native       string              `json:"native,omitempty"`
	TOMLTables   []tomlPersonalTable `json:"tomlTables,omitempty"`
	EmptyFile    *string             `json:"emptyFile,omitempty"`
	Present      bool                `json:"present"`
	EmptyParents map[string]string   `json:"emptyParents,omitempty"`
}

// legacyRetentionOriginal is the version 9 layout: one retention key per
// target.
type legacyRetentionOriginal struct {
	Key string `json:"key"`
	globalSettingOriginal
}

// personalNativeKeys lists the native keys the personal portable fields
// of the settings specs write for target. Removal restores their
// originals.
func personalNativeKeys(target string, f globalSettingsFile, settings []spec.Entry) []string {
	keys := voiceNativeKeys(target, f, settings)
	if f.retention != "" && slices.ContainsFunc(settings, func(entry spec.Entry) bool {
		_, present := entry.Meta[sessionRetentionField]
		return present && entry.EmitsTo(target)
	}) {
		keys = append(keys, f.retention)
	}
	return keys
}

// mergeGlobalPersonal merges the settings of target and records, per
// native key a personal field writes, what the key held before. A key
// no longer written gets that value back when sync's own value is still
// there.
func mergeGlobalPersonal(path, format string, base []byte, want []globalSetting, previous map[string]any, originals map[string]globalSettingOriginal, parents jsonParents, personal []string, target string, warn io.Writer) (globalSettingsMerge, map[string]globalSettingOriginal, error) {
	m, next, err := mergeGlobalPersonalInner(path, format, base, want, previous, originals, parents, personal, target, warn)
	if err != nil || target != "qwen" {
		return m, next, err
	}
	data := m.data
	if data == nil {
		data = base
		if data == nil {
			data, err = os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return m, next, fmt.Errorf("read %s: %w", path, err)
			}
		}
	}
	values, err := settingsValues(path, format, data)
	if err != nil {
		return m, next, err
	}
	noteQwenVoiceModel(values, want)
	return m, next, nil
}

func mergeGlobalPersonalInner(path, format string, base []byte, want []globalSetting, previous map[string]any, originals map[string]globalSettingOriginal, parents jsonParents, personal []string, target string, warn io.Writer) (globalSettingsMerge, map[string]globalSettingOriginal, error) {
	if len(personal) == 0 && len(originals) == 0 {
		m, err := mergeGlobalSettingsWithLists(path, format, base, want, previous, &parents, globalTargets[target].settings.itemLists)
		return m, nil, err
	}
	data := base
	if data == nil {
		var err error
		data, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return globalSettingsMerge{}, nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	values, err := settingsValues(path, format, data)
	if err != nil {
		return globalSettingsMerge{}, nil, err
	}
	portableKeys := slices.Clone(personal)
	for _, setting := range want {
		if slices.ContainsFunc(portableKeys, func(key string) bool { return settingsKeysOverlap(key, setting.key) }) {
			personal = append(personal, setting.key)
		}
	}
	var replacements []string
	for _, setting := range want {
		for key := range originals {
			if key != setting.key && settingsKeysOverlap(key, setting.key) {
				replacements = append(replacements, setting.key)
				break
			}
		}
	}
	if len(replacements) > 0 {
		// Release the old ownership shape before capturing its replacement,
		// so a parent override cannot swallow the original child values.
		staying := slices.DeleteFunc(slices.Clone(want), func(setting globalSetting) bool {
			return slices.Contains(replacements, setting.key)
		})
		stayingPersonal := slices.DeleteFunc(slices.Clone(personal), func(key string) bool {
			return slices.Contains(replacements, key)
		})
		first, remaining, err := mergeGlobalPersonalInner(path, format, data, staying, previous, originals, parents, stayingPersonal, target, warn)
		if err != nil {
			return first, nil, err
		}
		released := first.data
		if released == nil {
			released = data
		}
		second, next, err := mergeGlobalPersonalInner(path, format, released, want, first.owned, remaining, jsonParents{created: first.parents}, slices.Concat(personal, replacements), target, warn)
		if second.data == nil {
			second.data = first.data
		}
		second.changes = append(first.changes, second.changes...)
		second.conflicts = append(first.conflicts, second.conflicts...)
		second.adopted = append(first.adopted, second.adopted...)
		return second, next, err
	}
	if target == "claude" {
		want = reuseClaudeLegacyVoice(values, want)
	}
	originals = maps.Clone(originals)
	retention := globalTargets[target].settings.retention
	portable := retention != "" && slices.Contains(personal, retention)
	if (target == "claude" || target == "codebuddy") && portable && slices.ContainsFunc(want, func(setting globalSetting) bool {
		return setting.key == retention && setting.field == sessionRetentionField
	}) {
		if have, present := settingsLookup(values, retention, format); present {
			days, number := have.(float64)
			if !number || days < 1 || math.Trunc(days) != days {
				want = slices.DeleteFunc(slices.Clone(want), func(setting globalSetting) bool { return setting.field == sessionRetentionField })
				previous = maps.Clone(previous)
				delete(previous, retention)
				delete(originals, retention)
				adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, "native cleanupPeriodDays is invalid and can pause cleanup; manual correction is required before applying portable retention")
			}
		}
	}
	if enabled, present := settingsLookup(values, "general.sessionRetention.enabled", format); retention != "" && target != "claude" && target != "codebuddy" && present && enabled == false {
		want = slices.DeleteFunc(slices.Clone(want), func(setting globalSetting) bool { return setting.field == sessionRetentionField })
		if portable {
			if have, present := settingsLookup(values, retention, format); present && sameSetting(have, previous[retention]) && !slices.ContainsFunc(want, func(setting globalSetting) bool { return setting.key == retention }) {
				want = append(want, globalSetting{target: target, key: retention, field: sessionRetentionField, value: have})
			}
			adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, "automatic session cleanup is disabled; its age limit stays unchanged")
		}
	}
	_, managedRetention := originals[retention]
	managedRetention = retention != "" && (portable || managedRetention)
	next := map[string]globalSettingOriginal{}
	modifiedWant := slices.Clone(want)
	var restored []string
	var nativeRestored []string
	keys := slices.Concat(personal, slices.Collect(maps.Keys(originals)))
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		original, recorded := originals[key]
		if slices.ContainsFunc(want, func(setting globalSetting) bool { return setting.key == key }) {
			if !recorded && slices.Contains(personal, key) {
				original, recorded = captureSettingOriginal(data, values, key, format, parents.created), true
			}
			if recorded {
				next[key] = original
			}
			continue
		}
		if !recorded {
			continue
		}
		if len(original.EmptyParents) > 0 {
			if parents.keep == nil {
				parents.keep = map[string]string{}
			}
			maps.Copy(parents.keep, original.EmptyParents)
		}
		if have, present := settingsLookup(values, key, format); present && sameSetting(have, previous[key]) && original.Present {
			if len(original.TOMLTables) > 0 {
				if original.Raw != "{}" {
					data, err = restoreTOMLTables(data, key, original.TOMLTables)
					if err != nil {
						return globalSettingsMerge{}, nil, fmt.Errorf("restore %s: %w", path, err)
					}
				}
				previous = maps.Clone(previous)
				delete(previous, key)
				nativeRestored = append(nativeRestored, key)
				continue
			}
			if original.Native != "" {
				if start, end, ok := tomlPersonalSpan(data, key, format); ok {
					text := restoreTOMLAssignment(original.Native, string(data[start:end]), key, previous[key])
					if end < len(data) && !strings.HasSuffix(text, "\n") {
						text += "\n"
					}
					data = append(append(append([]byte(nil), data[:start]...), text...), data[end:]...)
					previous = maps.Clone(previous)
					delete(previous, key)
					nativeRestored = append(nativeRestored, key)
					continue
				}
			}
			decoded, err := settingsValues(path, "json", []byte(`{"value":`+original.Raw+`}`))
			if err != nil {
				return globalSettingsMerge{}, nil, fmt.Errorf("restore %s: %w", path, err)
			}
			modifiedWant = append(modifiedWant, globalSetting{key: key, value: decoded["value"]})
			restored = append(restored, key)
		}
	}
	mergeBase := base
	if len(nativeRestored) > 0 {
		mergeBase = data
	}
	m, err := mergeGlobalSettingsWithLists(path, format, mergeBase, modifiedWant, previous, &parents, globalTargets[target].settings.itemLists)
	if err != nil {
		return m, nil, err
	}
	if len(nativeRestored) > 0 && m.data == nil {
		m.data = data
	}
	for _, key := range nativeRestored {
		m.changes = append(m.changes, "restore "+key)
	}
	if m.data != nil && strings.TrimSpace(string(m.data)) == "" {
		for _, original := range originals {
			if original.EmptyFile != nil {
				m.data = []byte(*original.EmptyFile)
				break
			}
		}
	}
	for _, key := range restored {
		after := m.data
		if after == nil {
			after = data
		}
		if start, end, ok := jsonKeyValueSpan(string(after), key); ok && format == "json" {
			m.data = append(append(append([]byte(nil), after[:start]...), originals[key].Raw...), after[end:]...)
		}
		delete(m.owned, key)
	}
	if managedRetention && m.data != nil {
		if err := warnShorterRetention(path, format, retention, values, m.data, warn); err != nil {
			return m, nil, err
		}
	}
	if len(next) == 0 {
		next = nil
	}
	return m, next, nil
}

// TOML keeps the whole assignment because its spacing and comments belong to the original key.
func captureSettingOriginal(data []byte, values map[string]any, key, format string, createdParents []string) globalSettingOriginal {
	original := globalSettingOriginal{EmptyParents: map[string]string{}}
	if (format == "toml" || nestedTOMLFormat(format)) && strings.TrimSpace(string(data)) == "" {
		empty := string(data)
		original.EmptyFile = &empty
	}
	if format == "json" {
		for _, parent := range jsonParentKeys(key) {
			if open, closing, count, present := jsonContainerSpan(string(data), parent); present && count == 0 && data[open] == '{' && !slices.Contains(createdParents, parent) {
				original.EmptyParents[parent] = string(data[open : closing+1])
			}
		}
		if start, end, ok := jsonKeyValueSpan(string(data), key); ok {
			original.Present, original.Raw = true, string(data[start:end])
		}
		return original
	}
	if have, present := settingsLookup(values, key, format); present {
		if raw, err := json.Marshal(have); err == nil {
			original.Present, original.Raw = true, string(raw)
			if format == grokTOMLFormat && !strings.Contains(key, ".") {
				if sections, err := grokTableSections(data, key); err == nil {
					for _, section := range sections {
						original.TOMLTables = append(original.TOMLTables, tomlPersonalTable{Text: string(data[section.start:section.end]), Before: section.before, Array: section.array, Occurrence: section.occurrence})
					}
				}
			} else if start, end, ok := tomlPersonalSpan(data, key, format); ok {
				original.Native = string(data[start:end])
			}
		}
	}
	return original
}

func jsonKeyValueSpan(text, key string) (int, int, bool) {
	open, err := jsonRootOpen(text)
	if err != nil {
		return 0, 0, false
	}
	start, err := findJSONPath(text, open, strings.Split(key, "."))
	if err != nil || start < 0 {
		return 0, 0, false
	}
	end, err := jsonValueEnd(text, start)
	return start, end, err == nil
}
