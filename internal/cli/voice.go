package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const voiceField = "voice"

// globalVoiceKeys names the native key one target's user settings file
// takes for each portable voice member. An empty key is not written, and
// notes says why, per member.
type globalVoiceKeys struct {
	enabled, mode, language string
	// modes translates a portable mode to the native value; a missing
	// mode is written as it is.
	modes map[string]string
	notes map[string]string
}

func voiceIssues(settings []spec.Entry) []validationIssue {
	var issues []validationIssue
	for _, entry := range settings {
		value, present := entry.Meta[voiceField]
		if !present {
			continue
		}
		_, problems := spec.ParseVoice(value)
		for _, problem := range problems {
			issues = append(issues, validationIssue{Path: entry.Path, Field: voiceField, Message: problem, entry: entry})
		}
	}
	return issues
}

// globalVoiceSetting resolves the voice members of the settings specs
// for target into native keys, later specs winning member by member.
func globalVoiceSetting(target string, f globalSettingsFile, settings []spec.Entry) []globalSetting {
	byMember := map[string]globalSetting{}
	var order []string
	present := false
	for _, entry := range settings {
		value, ok := entry.Meta[voiceField]
		if !ok || !entry.EmitsTo(target) {
			continue
		}
		voice, problems := spec.ParseVoice(value)
		if len(problems) > 0 {
			continue
		}
		present = true
		if f.voice == nil {
			continue
		}
		set := func(member, key string, native any) {
			if _, seen := byMember[member]; !seen {
				order = append(order, member)
			}
			byMember[member] = globalSetting{target: target, key: key, field: voiceField + "." + member, source: entry.Path, value: native}
		}
		if voice.Enabled != nil {
			set("enabled", f.voice.enabled, *voice.Enabled)
		}
		if voice.Mode != "" {
			native := voice.Mode
			if translated, ok := f.voice.modes[voice.Mode]; ok {
				native = translated
			}
			set("mode", f.voice.mode, native)
		}
		if voice.Language != nil {
			set("language", f.voice.language, *voice.Language)
		}
	}
	if !present {
		return nil
	}
	if f.voice == nil {
		adapters.NoteSettingsFieldNoOp(target, voiceField, 1, voiceTargetNoOpReason(target))
		return nil
	}
	var out []globalSetting
	for _, member := range order {
		s := byMember[member]
		if s.key == "" {
			reason := f.voice.notes[member]
			if reason == "" {
				reason = "no native key for this voice member"
			}
			adapters.NoteSettingsFieldNoOp(target, s.field, 1, reason)
			continue
		}
		out = append(out, s)
	}
	return out
}

// voiceNativeKeys lists the native keys the voice mapping writes for
// target from the members the settings specs set.
func voiceNativeKeys(target string, f globalSettingsFile, settings []spec.Entry) []string {
	if f.voice == nil {
		return nil
	}
	var keys []string
	for _, entry := range settings {
		value, ok := entry.Meta[voiceField]
		if !ok || !entry.EmitsTo(target) {
			continue
		}
		voice, problems := spec.ParseVoice(value)
		if len(problems) > 0 {
			continue
		}
		for _, member := range []struct {
			set bool
			key string
		}{{voice.Enabled != nil, f.voice.enabled}, {voice.Mode != "", f.voice.mode}, {voice.Language != nil, f.voice.language}} {
			if member.set && member.key != "" {
				keys = append(keys, member.key)
			}
		}
	}
	return keys
}
