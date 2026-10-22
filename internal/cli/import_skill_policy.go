package cli

import "gopkg.in/yaml.v3"

// windsurfSkillFields is the Agent Skills baseline plus Devin's triggers,
// which the native file always states, so import takes the policy from it.
var windsurfSkillFields = specFields{keys: fieldsOf("name", "description", "license", "triggers").keys, preserve: preserveWindsurfSkillTriggers}

// crushSkillFields is the Agent Skills baseline plus Crush's palette flag,
// which the native file states beside disable-model-invocation.
var crushSkillFields = specFields{keys: fieldsOf("name", "description", "license", "user-invocable").keys, preserve: preserveCrushSkillPolicy}

// preserveWindsurfSkillTriggers drops the spec's x-windsurf.triggers on
// import. The native file states the policy, as triggers the import
// already read or as Devin's default, so a kept override would undo a
// change made in the tool. Sync never shares a skills folder that
// carries the override (AAI-102), so the file always reflects it.
func preserveWindsurfSkillTriggers(existing, _ *yaml.Node) {
	removeScopedKeys(existing, "x-windsurf", "triggers")
}

// preserveCrushSkillPolicy drops the spec's x-crush invocation fields on
// import, for the same reason: the native file states them.
func preserveCrushSkillPolicy(existing, _ *yaml.Node) {
	removeScopedKeys(existing, "x-crush", "disable-model-invocation", "user-invocable")
}

// removeScopedKeys removes keys from existing's scoped block, and the
// block once empty. An alias to a removed node gets its own copy first,
// so the spec stays readable.
func removeScopedKeys(existing *yaml.Node, block string, keys ...string) {
	index := mappingIndex(existing, block)
	if index < 0 {
		return
	}
	custom := existing.Content[index+1]
	if custom.Kind != yaml.MappingNode {
		return
	}
	for _, key := range keys {
		if i := mappingIndex(custom, key); i >= 0 {
			detachDependents(existing, custom.Content[i+1])
			removeMappingKey(custom, key)
		}
	}
	if len(custom.Content) == 0 {
		removeMappingKey(existing, block)
	}
}
