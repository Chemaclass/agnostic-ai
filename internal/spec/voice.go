package spec

import (
	"maps"
	"slices"
)

// VoiceModes are the portable dictation modes: hold to talk, or tap to
// start and tap again to stop.
var VoiceModes = []string{"hold", "tap"}

// Voice is a settings spec's portable voice dictation field. A nil or
// empty member was not set.
type Voice struct {
	Enabled  *bool
	Mode     string
	Language *string
}

// ParseVoice reads the voice mapping of a settings spec and returns one
// problem per invalid member.
func ParseVoice(value any) (Voice, []string) {
	fields, ok := value.(map[string]any)
	if !ok {
		return Voice{}, []string{"voice must be a mapping of enabled, mode, and language"}
	}
	var v Voice
	var problems []string
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		switch member := fields[key]; key {
		case "enabled":
			enabled, ok := member.(bool)
			if !ok {
				problems = append(problems, "voice.enabled must be true or false")
				continue
			}
			v.Enabled = &enabled
		case "mode":
			mode, ok := member.(string)
			if !ok || !slices.Contains(VoiceModes, mode) {
				problems = append(problems, "voice.mode must be hold or tap")
				continue
			}
			v.Mode = mode
		case "language":
			language, ok := member.(string)
			if !ok {
				problems = append(problems, "voice.language must be a string")
				continue
			}
			v.Language = &language
		default:
			problems = append(problems, "voice takes enabled, mode, and language; set voice."+key+" under x-<target>")
		}
	}
	return v, problems
}
