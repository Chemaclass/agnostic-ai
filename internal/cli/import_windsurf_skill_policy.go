package cli

import "gopkg.in/yaml.v3"

// windsurfSkillFields is the Agent Skills baseline plus Devin's triggers,
// which the native file always states, so import takes the policy from it.
var windsurfSkillFields = specFields{keys: fieldsOf("name", "description", "license", "triggers").keys, preserve: preserveWindsurfSkillTriggers}

// preserveWindsurfSkillTriggers drops the spec's x-windsurf.triggers on
// import. The native file states the policy, as triggers the import
// already read or as Devin's default, so a kept override would undo a
// change made in the tool. Sync never shares a skills folder that
// carries the override (AAI-102), so the file always reflects it.
func preserveWindsurfSkillTriggers(existing, _ *yaml.Node) {
	index := mappingIndex(existing, "x-windsurf")
	if index < 0 {
		return
	}
	custom := existing.Content[index+1]
	if custom.Kind != yaml.MappingNode {
		return
	}
	removeMappingKey(custom, "triggers")
	if len(custom.Content) == 0 {
		removeMappingKey(existing, "x-windsurf")
	}
}
