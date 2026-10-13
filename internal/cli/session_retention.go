package cli

import (
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const sessionRetentionField = "session-retention-days"

type globalRetentionOriginal struct {
	Key          string            `json:"key"`
	Raw          string            `json:"raw,omitempty"`
	Present      bool              `json:"present"`
	EmptyParents map[string]string `json:"emptyParents,omitempty"`
}

func sessionRetentionIssues(settings []spec.Entry) []validationIssue {
	var issues []validationIssue
	for _, entry := range settings {
		if value, present := entry.Meta[sessionRetentionField]; present {
			if _, valid := spec.SessionRetentionDays(value); !valid {
				issues = append(issues, validationIssue{Path: entry.Path, Field: sessionRetentionField, Message: "session-retention-days must be an integer of 1 or more", entry: entry})
			}
		}
	}
	return issues
}

func globalRetentionSetting(target string, f globalSettingsFile, settings []spec.Entry) []globalSetting {
	var out []globalSetting
	for _, entry := range settings {
		value, present := entry.Meta[sessionRetentionField]
		if !present || !entry.EmitsTo(target) {
			continue
		}
		days, valid := spec.SessionRetentionDays(value)
		if !valid {
			continue
		}
		if f.retention == "" {
			reason := "no documented retention setting for session files"
			switch target {
			case "kilo":
				reason = "session retention needs a separate design for Kilo's deletion switch"
			case "factory":
				reason = "session retention is controlled by the organization"
			}
			adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, reason)
			continue
		}
		var native any = days
		if target != "claude" {
			native = strconv.FormatUint(days, 10) + "d"
		}
		out = []globalSetting{{target: target, key: f.retention, field: sessionRetentionField, source: entry.Path, value: native}}
	}
	return out
}

func hasSessionRetention(settings []spec.Entry, target string) bool {
	return slices.ContainsFunc(settings, func(entry spec.Entry) bool {
		_, present := entry.Meta[sessionRetentionField]
		return present && entry.EmitsTo(target)
	})
}

func mergeGlobalRetention(path, format string, base []byte, want []globalSetting, previous map[string]any, original *globalRetentionOriginal, key, target string, portable bool, warn io.Writer) (globalSettingsMerge, *globalRetentionOriginal, error) {
	if key == "" && original == nil {
		m, err := mergeGlobalSettings(path, format, base, want, previous)
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
	if target == "claude" && portable && slices.ContainsFunc(want, func(setting globalSetting) bool { return setting.key == key && setting.field == sessionRetentionField }) {
		if have, present := settingsLookup(values, key, format); present {
			days, number := have.(float64)
			if !number || days < 1 || math.Trunc(days) != days {
				want = slices.DeleteFunc(slices.Clone(want), func(setting globalSetting) bool { return setting.field == sessionRetentionField })
				previous = maps.Clone(previous)
				delete(previous, key)
				original = nil
				adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, "native cleanupPeriodDays is invalid and can pause cleanup; manual correction is required before applying portable retention")
			}
		}
	}
	if enabled, present := settingsLookup(values, "general.sessionRetention.enabled", format); target != "claude" && present && enabled == false {
		want = slices.DeleteFunc(slices.Clone(want), func(setting globalSetting) bool { return setting.field == sessionRetentionField })
		if portable {
			if have, present := settingsLookup(values, key, format); present && sameSetting(have, previous[key]) && !slices.ContainsFunc(want, func(setting globalSetting) bool { return setting.key == key }) {
				want = append(want, globalSetting{target: target, key: key, field: sessionRetentionField, value: have})
			}
			adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, "automatic session cleanup is disabled; its age limit stays unchanged")
		}
	}
	active := slices.ContainsFunc(want, func(setting globalSetting) bool { return setting.key == key })
	if original == nil && portable && active {
		original = &globalRetentionOriginal{Key: key, EmptyParents: map[string]string{}}
		parts := strings.Split(key, ".")
		for i := 1; i < len(parts); i++ {
			parent := strings.Join(parts[:i], ".")
			if open, closing, count, present := jsonContainerSpan(string(data), parent); present && count == 0 && data[open] == '{' {
				original.EmptyParents[parent] = string(data[open : closing+1])
			}
		}
		if start, end, ok := retentionValueSpan(string(data), key); ok {
			original.Present, original.Raw = true, string(data[start:end])
		}
	}
	managedRetention := portable || original != nil
	modifiedWant := slices.Clone(want)
	restore := false
	if original != nil && !active {
		if have, present := settingsLookup(values, original.Key, format); present && sameSetting(have, previous[original.Key]) && original.Present {
			var value any
			decoded, err := settingsValues(path, "json", []byte(`{"value":`+original.Raw+`}`))
			if err != nil {
				return globalSettingsMerge{}, nil, fmt.Errorf("restore %s: %w", path, err)
			}
			value = decoded["value"]
			modifiedWant = append(modifiedWant, globalSetting{key: original.Key, value: value})
			restore = true
		}
	}
	var keep map[string]string
	if original != nil && !active {
		keep = original.EmptyParents
	}
	m, err := mergeGlobalSettingsKeeping(path, format, base, modifiedWant, previous, keep)
	if err != nil {
		return m, nil, err
	}
	if restore {
		after := m.data
		if after == nil {
			after = data
		}
		if start, end, ok := retentionValueSpan(string(after), original.Key); ok {
			m.data = append(append(append([]byte(nil), after[:start]...), original.Raw...), after[end:]...)
		}
		delete(m.owned, original.Key)
	}
	if managedRetention && m.data != nil {
		if err := warnShorterRetention(path, format, key, values, m.data, warn); err != nil {
			return m, nil, err
		}
	}
	if !active {
		original = nil
	}
	return m, original, nil
}

func warnShorterRetention(path, format, key string, before map[string]any, after []byte, warn io.Writer) error {
	if enabled, present := settingsLookup(before, "general.sessionRetention.enabled", format); present && enabled == false {
		return nil
	}
	values, err := settingsValues(path, format, after)
	if err != nil {
		return err
	}
	if enabled, present := settingsLookup(values, "general.sessionRetention.enabled", format); present && enabled == false {
		return nil
	}
	age := func(settings map[string]any) (float64, bool) {
		if value, present := settingsLookup(settings, key, format); present {
			return nativeRetentionDays(value)
		}
		return 30, true
	}
	oldDays, oldOK := age(before)
	newDays, newOK := age(values)
	if oldOK && newOK && newDays < oldDays {
		if _, err := fmt.Fprintf(warn, "warning: %s: lowering session retention from %g to %g days deletes older sessions\n", path, oldDays, newDays); err != nil {
			return fmt.Errorf("write session retention warning: %w", err)
		}
	}
	return nil
}

func retentionValueSpan(text, key string) (int, int, bool) {
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

func nativeRetentionDays(value any) (float64, bool) {
	if days, valid := spec.SessionRetentionDays(value); valid {
		return float64(days), true
	}
	if number, ok := value.(float64); ok {
		return number, number > 0
	}
	text, ok := value.(string)
	if !ok || len(text) < 2 {
		return 0, false
	}
	count, err := strconv.ParseFloat(text[:len(text)-1], 64)
	if err != nil || count <= 0 {
		return 0, false
	}
	units := map[byte]float64{'s': 1.0 / 86400, 'm': 1.0 / 1440, 'h': 1.0 / 24, 'd': 1, 'w': 7}
	unit, valid := units[text[len(text)-1]]
	return count * unit, valid
}
