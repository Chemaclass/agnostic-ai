package emit

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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

// SettingsCustomObject returns the object at `x-<target>.<key>` on one
// settings spec, and whether it was there. Adapters use it for the one
// key they merge with their own translated output rather than letting
// the general passthrough set it.
func SettingsCustomObject(entry spec.Entry, target, key string) (map[string]any, bool) {
	custom, _ := CustomTargetMeta(entry.Meta, target)
	if custom == nil {
		return nil, false
	}
	value, ok := custom[key].(map[string]any)
	return value, ok
}

// SettingsCustomList returns the array at `x-<target>.<key>` on one
// settings spec. The list counterpart to SettingsCustomObject.
func SettingsCustomList(entry spec.Entry, target, key string) []any {
	custom, _ := CustomTargetMeta(entry.Meta, target)
	if custom == nil {
		return nil
	}
	value, _ := custom[key].([]any)
	return value
}

// MergeSettingsCustomKeys sets every key SettingsCustomKeys returns
// onto the adapter's managed map, so the hatch rides the same merge
// the adapter already performs for its own keys.
//
// Same-shaped values merge rather than replace: two lists union in
// order, two objects merge key by key and recurse, and a scalar is
// replaced outright since it has no parts to keep. So a translated
// deny rule cannot vanish because an author wrote one of their own
// under `x-<target>`; both end up in the list. That is the rule the
// four hand-wired hatches already follow, now the general one (#966).
//
// A shape that cannot merge (a list against a string, an object
// against a list) keeps the hatch value, the precedence `ResolveMeta`
// gives a frontmatter override, and raises a coverage note so the
// replacement is never silent.
func MergeSettingsCustomKeys(keys map[string]any, settings []spec.Entry, target string, exclude ...string) {
	for key, value := range SettingsCustomKeys(settings, target, exclude...) {
		keys[key] = MergeSettingsCustomValue(target, key, keys[key], value)
	}
}

// MergeSettingsCustomValue merges one `x-<target>` value onto the
// value the adapter already produced for the same key, by the rule
// MergeSettingsCustomKeys documents, and notes a shape conflict.
// Exported for the adapters that write through an ordered JSON
// document instead of a plain map.
func MergeSettingsCustomValue(target, key string, managed, custom any) any {
	if managed == nil {
		return custom
	}
	merged, clean := mergeSettingsValues(managed, custom)
	if !clean {
		// The field reads "translated <key>", not "<key>": the key
		// itself does reach the file, carrying the hatch value. What
		// has no effect is the half this tool produced for it.
		NoteFieldNoOp(target, spec.KindSettings, "translated "+key, 1, settingsCustomConflictReason(target, key))
	}
	return merged
}

// settingsCustomConflictReason names the hatch key, since the note's
// own sentence cannot say which half of a conflict was kept.
func settingsCustomConflictReason(target, key string) string {
	return fmt.Sprintf("x-%s.%s holds a different shape, so the hatch value replaces it instead of merging with it", target, key)
}

// mergeSettingsValues merges one hatch value onto one managed value
// and reports whether every part of it merged cleanly. An unmergeable
// part takes the hatch value and turns the report false, so the caller
// notes once for the whole key rather than once per nested collision.
func mergeSettingsValues(managed, custom any) (any, bool) {
	managedList, managedIsList := settingsValueList(managed)
	customList, customIsList := settingsValueList(custom)
	if managedIsList && customIsList {
		return unionSettingsLists(managedList, customList), true
	}
	managedMap, managedIsMap := managed.(map[string]any)
	customMap, customIsMap := custom.(map[string]any)
	if managedIsMap && customIsMap {
		return mergeSettingsMaps(managedMap, customMap)
	}
	if managedIsList || customIsList || managedIsMap || customIsMap {
		return custom, false
	}
	// Two scalars: there is nothing to keep from the managed one.
	return custom, true
}

// mergeSettingsMaps returns a new map holding every key of both, the
// managed one first so its order-independent siblings survive, and
// merges the keys they share. The inputs are never mutated: the
// managed map can be a value an adapter still holds a reference to.
func mergeSettingsMaps(managed, custom map[string]any) (map[string]any, bool) {
	out := make(map[string]any, len(managed)+len(custom))
	for k, v := range managed {
		out[k] = v
	}
	clean := true
	for k, v := range custom {
		existing, ok := out[k]
		if !ok || existing == nil {
			out[k] = v
			continue
		}
		merged, ok := mergeSettingsValues(existing, v)
		if !ok {
			clean = false
		}
		out[k] = merged
	}
	return out, clean
}

// unionSettingsLists appends the hatch entries the managed list does
// not already hold, keeping the managed order first so a re-sync of
// the same specs produces the same file. Entries compare on their JSON
// form, so a string, a number, and an object all de-duplicate.
func unionSettingsLists(managed, custom []any) []any {
	out := make([]any, 0, len(managed)+len(custom))
	seen := make(map[string]bool, len(managed)+len(custom))
	for _, list := range [][]any{managed, custom} {
		for _, v := range list {
			key := settingsValueKey(v)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}

// settingsValueKey renders one list entry as the string the union
// de-duplicates on. JSON, so `[]string{"a"}` and `[]any{"a"}` collapse
// the way they will once both are written to the same file.
func settingsValueKey(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(raw)
}

// settingsValueList reports whether v is a list and returns it as
// `[]any`. Managed values arrive as `[]string` from the adapters and
// hatch values as `[]any` from the YAML decoder, so both spellings of
// the same list have to meet here.
func settingsValueList(v any) ([]any, bool) {
	switch list := v.(type) {
	case nil:
		return nil, false
	case []any:
		return list, true
	case []string:
		out := make([]any, len(list))
		for i, s := range list {
			out[i] = s
		}
		return out, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}
