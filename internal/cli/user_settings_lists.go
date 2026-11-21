package cli

import (
	"fmt"
	"maps"
	"os"
	"slices"
)

func mergeUserSettingsWithLists(path, format string, base []byte, want []userSetting, previous map[string]any, parents *jsonParents, lists []string) (userSettingsMerge, error) {
	if len(lists) == 0 {
		return mergeUserSettingsKeeping(path, format, base, want, previous, parents)
	}
	wantLists, previousLists := map[string][]string{}, map[string][]string{}
	keys := slices.Clone(want)
	owned := maps.Clone(previous)
	for _, key := range lists {
		for _, setting := range want {
			if setting.key == key {
				items, err := userSettingsList(path, key, setting.value)
				if err != nil {
					return userSettingsMerge{}, err
				}
				wantLists[key] = items
			}
		}
		if value, present := previous[key]; present {
			items, err := userSettingsList(path, key, value)
			if err != nil {
				return userSettingsMerge{}, err
			}
			previousLists[key] = items
		}
		keys = slices.DeleteFunc(keys, func(setting userSetting) bool { return setting.key == key })
		delete(owned, key)
	}
	data := base
	if data == nil {
		read, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return userSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
		if data == nil {
			data = []byte{}
		}
	}
	m, err := mergeUserSettingsKeeping(path, format, data, keys, owned, parents)
	if err != nil {
		return m, err
	}
	if m.data != nil {
		data = m.data
	}
	var listParents jsonParents
	if parents != nil {
		listParents = jsonParents{created: joinParents(parents.created, m.parents)}
	}
	merged, err := mergeUserStringLists(path, data, wantLists, previousLists, lists, "", listParents)
	if err != nil {
		return m, err
	}
	if merged.data != nil {
		m.data = merged.data
	}
	m.changes = append(m.changes, merged.changes...)
	m.adopted = append(m.adopted, merged.adopted...)
	m.parents = joinParents(m.parents, merged.parents)
	for key, items := range merged.owned {
		m.owned[key] = items
	}
	return m, nil
}

func userSettingsList(path, key string, value any) ([]string, error) {
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
