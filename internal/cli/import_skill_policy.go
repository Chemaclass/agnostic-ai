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
// import, for the same reason: the native file states them. A native file
// that keeps user-invocable but lets the model start the skill also drops
// the portable restriction. A file with neither field may be a copy
// shared with other targets, so it leaves the restriction alone.
func preserveCrushSkillPolicy(existing, imported *yaml.Node) {
	removeScopedKeys(existing, "x-crush", "disable-model-invocation", "user-invocable")
	if crushStatesModelInvocation(imported) {
		if i := mappingIndex(existing, "disable-model-invocation"); i >= 0 {
			detachDependents(existing, existing.Content[i+1])
			removeMappingKey(existing, "disable-model-invocation")
		}
	}
}

// crushStatesModelInvocation reports whether an imported Crush skill
// carries user-invocable without disable-model-invocation: true, so the
// native file lets the model start it.
func crushStatesModelInvocation(imported *yaml.Node) bool {
	if i := mappingIndex(imported, "disable-model-invocation"); i >= 0 && imported.Content[i+1].Value == "true" {
		return false
	}
	i := mappingIndex(imported, "x-crush")
	if i < 0 {
		return false
	}
	custom := imported.Content[i+1]
	if j := mappingIndex(custom, "disable-model-invocation"); j >= 0 && custom.Content[j+1].Value == "true" {
		return false
	}
	return mappingIndex(custom, "user-invocable") >= 0
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
