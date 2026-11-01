package cli

import "gopkg.in/yaml.v3"

// specFields names the frontmatter a target writes for one spec kind.
// Import uses it to tell two cases apart that look identical on disk: a
// key missing from a native file because the format has nowhere to put
// it, which the spec keeps, and a key missing because the user deleted
// it, which the spec must lose. A target that round-trips everything
// declares allSpecFields and gets a plain overwrite.
type specFields struct {
	keys             map[string]bool
	scopedNative     map[string]bool
	scopedReconciled bool // importer already reconciles target metadata before writing
	all              bool
	omitted          map[string]bool
	preserve         func(existing, imported *yaml.Node)
}

// allSpecFields marks a native format that carries every key a spec can
// declare, so nothing is ever carried over.
var allSpecFields = specFields{all: true}

var reconciledAllSpecFields = specFields{all: true, scopedReconciled: true}

// fieldsOf builds the set a target's emitter writes for one kind. The
// lists come from syncing a spec that declares every portable key and
// reading back what each adapter wrote; TestSpecFields_MatchWhatSyncEmits
// re-derives them so an adapter that grows a field fails the build.
func fieldsOf(keys ...string) specFields {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return specFields{keys: set}
}

// expresses reports whether the target's native file has a home for key.
func (f specFields) expresses(key string) bool {
	if key == "can" {
		return !f.omitted[key] && (f.all || f.keys["tools"] || f.keys["allowed-tools"])
	}
	return !f.omitted[key] && (f.all || f.keys[key])
}

// Skill field sets match each target's native frontmatter.
var (
	vibeSkillFields      = fieldsOf("name", "description", "license", "compatibility", "metadata", "allowed-tools", "disable-model-invocation", "user-invocable")
	defaultSkillFields   = fieldsOf("name", "description", "license")
	opencodeSkillFields  = specFields{keys: fieldsOf("name", "description", "license", "disable-model-invocation", "metadata").keys, preserve: preserveOpencodeSkillPolicy}
	copilotSkillFields   = fieldsOf("name", "description", "license", "argument-hint", "disable-model-invocation")
	cursorSkillFields    = fieldsOf("name", "description", "license", "paths", "disable-model-invocation", "icon", "color", "metadata")
	flattenedSkillFields = fieldsOf("name", "description")
)

// Agents: one list per target that imports an agent spec over an
// existing one.
var (
	claudeAgentFields      = specFields{all: true, scopedReconciled: true, omitted: map[string]bool{"readonly": true, "x-claude": true}, preserve: preserveClaudeReadonly}
	clineAgentFields       = specFields{keys: fieldsOf("name", "description").keys, scopedNative: map[string]bool{"tools": true, "skills": true, "providerId": true, "modelId": true, "maxIterations": true}}
	cursorAgentFields      = specFields{keys: fieldsOf("name", "description", "model", "background").keys, scopedReconciled: true}
	copilotAgentFields     = fieldsOf("name", "description", "tools", "model")
	antigravityAgentFields = fieldsOf("name", "description")
	kiroAgentFields        = fieldsOf("name", "description", "model", "tools")
	qoderAgentFields       = specFields{keys: fieldsOf("name", "description", "model", "tools", "color", "effort", "memory", "maxTurns", "background").keys, scopedReconciled: true}
	gooseAgentFields       = fieldsOf("name", "description", "model")
	openhandsAgentFields   = specFields{keys: fieldsOf("name", "description", "model").keys, scopedNative: map[string]bool{"skills": true}}
	traeAgentFields        = fieldsOf("name", "description", "tools")
	windsurfAgentFields    = fieldsOf("name", "description", "model", "allowed-tools")
	factoryAgentFields     = fieldsOf("name", "description", "model", "tools", "effort", "reasoningEffort")
	// slicedAgentFields covers an agent recovered from an `##` section of
	// a shared instructions file (warp, goose), where the heading and body
	// are all the format holds.
	slicedAgentFields = fieldsOf("name", "description", "tags")
)

// cursorCommandFields covers `.cursor/commands/<name>.md`, which carries
// a description and a model and drops the rest.
var cursorCommandFields = fieldsOf("description", "model")

// factoryCommandFields covers `.factory/commands/<name>.md`.
var factoryCommandFields = fieldsOf("description", "argument-hint")

// flattenedKindFields picks the field set for a spec recovered from a
// flattened instructions file by its `agent-` / `skill-` filename prefix
// (copilot's `.github/instructions/`, the shared rules-directory
// importers). A rule keeps a plain overwrite: its translator drops a
// catch-all `globs` and an empty `description` on purpose (#429), so the
// spec has to follow.
func flattenedKindFields(kind string) specFields {
	switch kind {
	case "agents":
		return slicedAgentFields
	case "skills":
		return flattenedSkillFields
	default:
		return allSpecFields
	}
}
