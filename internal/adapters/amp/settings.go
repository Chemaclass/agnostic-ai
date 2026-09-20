package amp

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// toolsDisableKey is the one tool key on Amp's settings surface:
// "Disable specific tools by name. Use `builtin:toolname` to disable
// only the built-in tool with that name while allowing an MCP server
// to provide a tool by the same name. Glob patterns using `*` are
// supported" (ampcode.com/docs/cli/settings, and the same sentence in
// ampcode.com/cli-settings.schema.json, which types it as an array of
// strings).
const toolsDisableKey = "amp.tools.disable"

// allowNoKeyReason explains why a portable allow list reaches nothing.
// Amp's schema declares twenty properties and not one of them admits a
// tool. The only allow-shaped construct on the whole surface is
// `amp.mcpPermissions`, and it matches MCP *servers* by command or
// url, not tools.
const allowNoKeyReason = "Amp's settings schema has no tool allow-list; amp.tools.disable is its only tool key and it only disables"

// askNoTierReason explains why a portable ask list reaches nothing.
// The vendor states the opposite posture outright: "By default, Amp
// does not ask for approval before running tools"
// (ampcode.com/docs/tools), and its permissions section repeats it
// before pointing at a plugin rather than a setting.
const askNoTierReason = "Amp does not ask for approval before running tools, so its settings have no ask tier; control tool use with a plugin"

// denyNoVocabularyReason explains why a portable deny list reaches
// nothing *yet*. The surface exists and the shape matches, but Amp
// publishes no enumeration of its tool names: "You can see Amp's
// builtin tools by running `amp tools list` in the CLI"
// (ampcode.com/docs/tools). Translating `Bash(rm:*)` into a guessed
// name would write a file Amp silently ignores, which reads as a
// green sync that enforced nothing. The names have to come from a
// real `amp tools list` run before any mapping lands here (#950).
const denyNoVocabularyReason = "Amp publishes its tool names only through `amp tools list`, so a portable rule has no name to translate to; set x-amp with `" + toolsDisableKey + "` in Amp's own spelling"

// modelNoKeyReason explains why a portable model reaches nothing.
// Amp routes models through the Dial and its workspace, not through a
// settings key; no property in cli-settings.schema.json selects one.
const modelNoKeyReason = "no model key in Amp's settings schema; Amp picks models through the Dial"

// emitSettingsFile writes (or merges into) `.amp/settings.json`, the
// workspace-tier settings file Amp finds as "the nearest
// `.amp/settings.json` or `.amp/settings.jsonc`, searched upward from
// your current working directory to the repository root"
// (ampcode.com/docs/cli/settings). MCP servers and settings specs
// share that one file, so they share one merge: two writes to the same
// path in one sync would each rewrite what the other just put there.
//
// Routes through emit.MergeJSONFile so every pre-existing key survives
// the sync. Only `amp.mcpServers` and the keys an author wrote under
// `x-amp` are ever set, and nothing is written at all when neither
// source contributes.
//
// Portable settings fields do not translate here. Amp is deny-only and
// its tool vocabulary is unpublished, so each portable field raises its
// own coverage note instead of a guess; see noteSettingsGaps.
func emitSettingsFile(sess *emit.Session, mcps, settings []spec.Entry, path string, dryRun bool) error {
	noteSettingsGaps(settings)
	keys := map[string]any{}
	if servers := buildMCPMap(mcps); len(servers) > 0 {
		keys[ampMCPKey] = servers
	}
	// `amp.mcpServers` is excluded from the settings hatch. It is
	// built from MCP specs, each of which already carries its own
	// `x-amp` block for the fields this project does not model, so a
	// blanket set from a settings spec would replace every server
	// those specs contributed rather than adding to them (#949).
	emit.MergeSettingsCustomKeys(keys, settings, target, ampMCPKey)
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}

// noteSettingsGaps records the portable settings fields that reach
// nothing on Amp, one note per field so each carries its own reason.
// They are different failure modes and collapsing them would hide the
// only one that is temporary: `deny` has a surface waiting for a tool
// vocabulary, while `allow`, `ask`, and `model` have no surface at all.
func noteSettingsGaps(settings []spec.Entry) {
	allow, deny, ask, model := 0, 0, 0, 0
	for _, entry := range settings {
		if value, _ := entry.Meta["model"].(string); value != "" {
			model++
		}
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		if len(emit.StringSlice(permissions["allow"])) > 0 {
			allow++
		}
		if len(emit.StringSlice(permissions["deny"])) > 0 {
			deny++
		}
		if len(emit.StringSlice(permissions["ask"])) > 0 {
			ask++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.allow", allow, allowNoKeyReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.deny", deny, denyNoVocabularyReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.ask", ask, askNoTierReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "model", model, modelNoKeyReason)
}
