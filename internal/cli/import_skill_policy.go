package cli

import "gopkg.in/yaml.v3"

// windsurfSkillFields is the Agent Skills baseline plus Devin's triggers,
// whose absence can also mean the folder is shared with another target.
var windsurfSkillFields = specFields{
	keys: fieldsOf("name", "description", "license", "triggers").keys,
	// This flag is native only when x-windsurf explicitly passes it through.
	scopedNative: map[string]bool{"disable-model-invocation": true},
	preserve:     preserveWindsurfSkillTriggers,
}

// crushSkillFields is the Agent Skills baseline plus Crush's palette flag,
// which the native file states beside disable-model-invocation.
var crushSkillFields = specFields{keys: fieldsOf("name", "description", "license", "user-invocable").keys, preserve: preserveCrushSkillPolicy}

func preserveWindsurfSkillTriggers(existing, _ *yaml.Node) {
	removeScopedKeys(existing, "x-windsurf", "triggers")
}

func preserveDedicatedWindsurfSkillPolicy(existing, imported *yaml.Node) {
	var meta map[string]any
	if existing.Decode(&meta) != nil {
		return
	}
	custom, _ := meta["x-windsurf"].(map[string]any)
	restricted := meta["disable-model-invocation"] == true || custom["disable-model-invocation"] == true
	removeScopedKeys(existing, "x-windsurf", "triggers", "disable-model-invocation")
	if !restricted || mappingIndex(imported, "disable-model-invocation") >= 0 {
		return
	}
	index := mappingIndex(imported, "x-windsurf")
	if index >= 0 {
		custom := imported.Content[index+1]
		if mappingIndex(custom, "triggers") >= 0 || mappingIndex(custom, "disable-model-invocation") >= 0 {
			return
		}
		custom.Content = append(custom.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "disable-model-invocation"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"})
		return
	}
	// Only this tool's permission changes; the portable restriction stays.
	override := &yaml.Node{}
	if override.Encode(map[string]any{"disable-model-invocation": false}) != nil {
		return
	}
	imported.Content = append(imported.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x-windsurf"}, override)
}

// preserveCrushSkillPolicy drops the spec's x-crush invocation fields on
// import, for the same reason: the native file states them. A native file
// that keeps user-invocable but lets the model start the skill lifts the
// restriction for Crush only, as x-crush.disable-model-invocation: false:
// one tool's file must not hand the skill to every tool's model. A file
// with neither field may be a copy shared with other targets, so it
// leaves the restriction alone.
func preserveCrushSkillPolicy(existing, imported *yaml.Node) {
	removeScopedKeys(existing, "x-crush", "disable-model-invocation", "user-invocable")
	if !crushStatesModelInvocation(imported) || mappingIndex(existing, "disable-model-invocation") < 0 {
		return
	}
	custom := imported.Content[mappingIndex(imported, "x-crush")+1]
	if mappingIndex(custom, "disable-model-invocation") < 0 {
		custom.Content = append(custom.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "disable-model-invocation"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"})
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
// block once empty. Aliases to the block or anything in it get their own
// copy first, and an alias block is resolved to one, so the spec stays
// readable and other aliases keep their values.
func removeScopedKeys(existing *yaml.Node, block string, keys ...string) {
	index := mappingIndex(existing, block)
	if index < 0 {
		return
	}
	custom := existing.Content[index+1]
	if custom.Kind == yaml.AliasNode {
		custom = detachImportOverride(custom)
	} else {
		detachDependents(existing, custom)
		custom = detachImportOverride(custom)
	}
	if custom.Kind != yaml.MappingNode {
		return
	}
	existing.Content[index+1] = custom
	for _, key := range keys {
		removeMappingKey(custom, key)
	}
	if len(custom.Content) == 0 {
		removeMappingKey(existing, block)
	}
}
