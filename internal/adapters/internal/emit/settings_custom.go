package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// SettingsCustomKeys returns the `x-<target>` block carried by the
// settings specs, merged across specs in source order so a later spec
// wins per key. It is the settings-kind counterpart to the passthrough
// agents, commands, skills, rules, hooks, and MCP servers already have:
// a target-specific key an author writes in the vendor's own spelling
// reaches the emitted file instead of being dropped without a word.
//
// Every target has keys this project declines to model. Factory's
// `sandbox` block is the worked example: kernel-enforced isolation
// whose `denyWrite` overrides `allowWrite`, with no `ask` tier and an
// egress filter (`network.allowedDomains`) that no portable field
// matches. Kilo has a sandbox too, and the two overlap on one boolean.
// That is a hatch, not a spec kind (#949).
//
// Keys named in exclude are skipped. Use it for a key the adapter
// already reads itself, where the hand-wired hatch merges the author's
// rules with the translated ones rather than replacing them; a blanket
// set would quietly change what that shipped hatch does.
func SettingsCustomKeys(settings []spec.Entry, target string, exclude ...string) map[string]any {
	out := map[string]any{}
	for _, entry := range settings {
		custom, keys := CustomTargetMeta(entry.Meta, target, exclude...)
		for _, key := range keys {
			out[key] = custom[key]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeSettingsCustomKeys sets every key SettingsCustomKeys returns
// onto the adapter's managed map, so the hatch rides the same merge
// the adapter already performs for its own keys.
//
// The hatch wins on a collision. An author who writes the vendor's own
// spelling under `x-<target>` is stating what that file should hold,
// the same precedence `ResolveMeta` gives a frontmatter override.
func MergeSettingsCustomKeys(keys map[string]any, settings []spec.Entry, target string, exclude ...string) {
	for key, value := range SettingsCustomKeys(settings, target, exclude...) {
		keys[key] = value
	}
}
