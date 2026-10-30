package cli

import (
	"fmt"
	"io"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func importedGlobalVoice(target string, f globalSettingsFile, values map[string]any) map[string]any {
	if f.voice == nil {
		return nil
	}
	voice := map[string]any{}
	for _, member := range []struct{ name, key string }{
		{"enabled", f.voice.enabled}, {"mode", f.voice.mode}, {"language", f.voice.language},
	} {
		if member.key == "" {
			continue
		}
		value, present := settingsLookup(values, member.key, f.format)
		if !present && target == "claude" && member.name == "enabled" {
			if _, hasVoice := values["voice"]; !hasVoice {
				value, present = values["voiceEnabled"]
			}
		}
		if !present {
			continue
		}
		if mode, ok := value.(string); member.name == "mode" && ok {
			for portable, native := range f.voice.modes {
				if mode == native {
					value = portable
					break
				}
			}
		}
		if _, problems := spec.ParseVoice(map[string]any{member.name: value}); len(problems) == 0 {
			voice[member.name] = value
		}
	}
	return voice
}

func mergeImportedVoice(voice map[string]any, sources map[string]string, target string, incoming map[string]any, warn io.Writer) error {
	for _, key := range []string{"enabled", "mode", "language"} {
		value, present := incoming[key]
		if !present {
			continue
		}
		if current, set := voice[key]; set {
			if !sameSetting(current, value) {
				if _, err := fmt.Fprintf(warn, "warning: %s: skipped voice.%s = %v; %s already set it to %v\n", target, key, value, sources[key], current); err != nil {
					return err
				}
			}
			continue
		}
		voice[key], sources[key] = value, target
	}
	return nil
}

func homeSetsVoice(settings []spec.Entry, target string) bool {
	return slices.ContainsFunc(settings, func(entry spec.Entry) bool {
		_, present := entry.Meta[voiceField]
		return present && entry.EmitsTo(target)
	})
}
