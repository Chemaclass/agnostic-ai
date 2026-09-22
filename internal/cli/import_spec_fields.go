package cli

// specFields names the frontmatter a target writes for one spec kind.
// Import uses it to tell two cases apart that look identical on disk: a
// key missing from a native file because the format has nowhere to put
// it, which the spec keeps, and a key missing because the user deleted
// it, which the spec must lose. A target that round-trips everything
// declares allSpecFields and gets a plain overwrite.
type specFields struct {
	keys map[string]bool
	all  bool
}

// allSpecFields marks a native format that carries every key a spec can
// declare, so nothing is ever carried over.
var allSpecFields = specFields{all: true}

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
func (f specFields) expresses(key string) bool { return f.all || f.keys[key] }

// Skills: every target writes the Agent Skills `name` and `description`
// and nothing else, except Claude (the format the spec fields are named
// after, so it round-trips) and Cursor (five documented optional keys).
var (
	defaultSkillFields = fieldsOf("name", "description")
	cursorSkillFields  = fieldsOf("name", "description", "paths", "disable-model-invocation", "icon", "color", "metadata")
)

// Agents: one list per target that imports an agent spec over an
// existing one.
var (
	clineAgentFields       = fieldsOf("name", "description")
	cursorAgentFields      = fieldsOf("name", "description", "model")
	copilotAgentFields     = fieldsOf("name", "description", "tools", "model")
	antigravityAgentFields = fieldsOf("name", "description")
	kiroAgentFields        = fieldsOf("name", "description", "model", "tools")
	qoderAgentFields       = fieldsOf("name", "description", "model", "tools", "color", "effort", "memory")
	gooseAgentFields       = fieldsOf("name", "description", "model")
	traeAgentFields        = fieldsOf("name", "description", "tools")
	windsurfAgentFields    = fieldsOf("name", "description", "model", "allowed-tools")
	// slicedAgentFields covers an agent recovered from an `##` section of
	// a shared instructions file (warp, goose), where the heading and body
	// are all the format holds.
	slicedAgentFields = fieldsOf("name", "description", "tags")
)

// cursorCommandFields covers `.cursor/commands/<name>.md`, which carries
// a description and a model and drops the rest.
var cursorCommandFields = fieldsOf("description", "model")

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
		return defaultSkillFields
	default:
		return allSpecFields
	}
}
