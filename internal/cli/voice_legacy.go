package cli

import "slices"

func reuseClaudeLegacyVoice(values map[string]any, want []userSetting) []userSetting {
	if _, present := settingsLookup(values, "voice.enabled", "json"); present {
		return want
	}
	enabled, present := values["voiceEnabled"].(bool)
	if !present {
		return want
	}
	return slices.DeleteFunc(slices.Clone(want), func(setting userSetting) bool {
		value, valid := setting.value.(bool)
		return setting.key == "voice.enabled" && setting.field == "voice.enabled" && valid && value == enabled
	})
}
