package cli

import (
	"fmt"
	"maps"
	"os"
	"slices"
)

func mergeGlobalSettingsWithLists(path, format string, base []byte, want []globalSetting, previous map[string]any, parents *jsonParents, lists []string) (globalSettingsMerge, error) {
	if len(lists) == 0 {
		return mergeGlobalSettingsKeeping(path, format, base, want, previous, parents)
	}
	wantLists, previousLists := map[string][]string{}, map[string][]string{}
	keys := slices.Clone(want)
	owned := maps.Clone(previous)
	for _, key := range lists {
		for _, setting := range want {
			if setting.key == key {
				items, err := globalSettingsList(path, key, setting.value)
				if err != nil {
					return globalSettingsMerge{}, err
				}
				wantLists[key] = items
			}
		}
		if value, present := previous[key]; present {
			items, err := globalSettingsList(path, key, value)
			if err != nil {
				return globalSettingsMerge{}, err
			}
			previousLists[key] = items
		}
		keys = slices.DeleteFunc(keys, func(setting globalSetting) bool { return setting.key == key })
		delete(owned, key)
	}
	data := base
	if data == nil {
		read, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return globalSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
		if data == nil {
			data = []byte{}
		}
	}
	m, err := mergeGlobalSettingsKeeping(path, format, data, keys, owned, parents)
	if err != nil {
		return m, err
	}
	if m.data != nil {
		data = m.data
	}
	merged, err := mergeGlobalStringLists(path, data, wantLists, previousLists, lists, "")
	if err != nil {
		return m, err
	}
	if merged.data != nil {
		m.data = merged.data
	}
	m.changes = append(m.changes, merged.changes...)
	m.adopted = append(m.adopted, merged.adopted...)
	for key, items := range merged.owned {
		m.owned[key] = items
	}
	return m, nil
}

func globalSettingsList(path, key string, value any) ([]string, error) {
	items, ok := jsonRoundTrip(value).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be a list of strings", path, key)
	}
	var out []string
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be a list of strings", path, key)
		}
		if !slices.Contains(out, text) {
			out = append(out, text)
		}
	}
	return out, nil
}
