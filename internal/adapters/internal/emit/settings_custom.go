package emit

import (
	"bytes"
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
//
// A value present under some other shape reads as absent, and raises a
// coverage note on the way out. It cannot do anything else: the key is
// excluded from the general merge precisely because this adapter reads
// it itself, and it cannot walk what is not an object. Silence was the
// bad outcome, not the fallback. An author who wrote a permission
// policy the tool could not read got the translated policy instead and
// heard nothing about it (#976).
func SettingsCustomObject(entry spec.Entry, target, key string) (map[string]any, bool) {
	raw, held := settingsCustomValue(entry, target, key)
	if !held {
		return nil, false
	}
	value, ok := raw.(map[string]any)
	if !ok {
		noteSettingsCustomShape(target, key, raw, "an object")
		return nil, false
	}
	return value, true
}

// SettingsCustomList returns the array at `x-<target>.<key>` on one
// settings spec. The list counterpart to SettingsCustomObject, with
// the same treatment of a value of the wrong shape.
func SettingsCustomList(entry spec.Entry, target, key string) []any {
	raw, held := settingsCustomValue(entry, target, key)
	if !held {
		return nil
	}
	value, ok := raw.([]any)
	if !ok {
		noteSettingsCustomShape(target, key, raw, "a list")
		return nil
	}
	return value
}

// settingsCustomValue returns the raw value at `x-<target>.<key>` and
// whether the author wrote the key at all. A key written with no value
// decodes to nil, which reads as absent: an empty line in YAML is not
// a policy, and noting it would punish a comment-out.
func settingsCustomValue(entry spec.Entry, target, key string) (any, bool) {
	custom, _ := CustomTargetMeta(entry.Meta, target)
	if custom == nil {
		return nil, false
	}
	value, held := custom[key]
	if !held || value == nil {
		return nil, false
	}
	return value, true
}

// noteSettingsCustomShape reports one hatch the adapter reads itself
// and could not use. The field reads `x-<target>.<key>`, not `<key>`:
// the translated half of that key is what reaches the file, and the
// half with no effect is the author's.
func noteSettingsCustomShape(target, key string, value any, want string) {
	NoteFieldNoOp(target, spec.KindSettings, fmt.Sprintf("x-%s.%s", target, key), 1, fmt.Sprintf(
		"the value is %s, and this key takes %s, so the hatch is skipped and the translated value is written in its place",
		settingsValueShape(value), want))
}

// settingsValueShape names a value's JSON shape for that note, in the
// vocabulary the spec format uses for the hatch.
func settingsValueShape(v any) string {
	switch v.(type) {
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return "a number"
	}
	if _, ok := settingsValueList(v); ok {
		return "a list"
	}
	if _, ok := settingsValueObject(v); ok {
		return "an object"
	}
	return "a scalar"
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

// MergeSettingsCustomRecordMap merges the `x-<target>.<key>` map onto the
// record map the adapter already put under the same key, by name rather
// than by field: a name only one side carries is kept, and a name both
// sides carry is taken from the hatch whole.
//
// A record is not a property bag. An MCP server's transport fields have
// to agree with each other or the vendor rejects the entry, so the
// general field-by-field merge produced `command` from the hatch beside
// `args` from the spec, which runs the author's binary with the previous
// binary's flags, and a `url` beside a `command`, which is no transport
// at all (#974).
//
// Union at the registry level is still what an author wants, so this is
// not a return to replacing the whole map: that was the #966 bug. The
// boundary is one level down. An adapter excludes the key from
// MergeSettingsCustomKeys and calls this instead, the way the four
// hand-wired permission hatches already do for their own key.
func MergeSettingsCustomRecordMap(keys map[string]any, settings []spec.Entry, target, key string) {
	custom, ok := SettingsCustomKeys(settings, target)[key]
	if !ok {
		return
	}
	managed, held := keys[key]
	if !held || managed == nil {
		keys[key] = custom
		return
	}
	managedMap, managedIsMap := managed.(map[string]any)
	customMap, customIsMap := custom.(map[string]any)
	if !managedIsMap || !customIsMap {
		NoteFieldNoOp(target, spec.KindSettings, "translated "+key, 1, settingsCustomConflictReason(target, key))
		keys[key] = custom
		return
	}
	out := make(map[string]any, len(managedMap)+len(customMap))
	for name, record := range managedMap {
		out[name] = record
	}
	for name, record := range customMap {
		out[name] = record
	}
	keys[key] = out
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
	managedDoc, managedIsObject := settingsValueObject(managed)
	customDoc, customIsObject := settingsValueObject(custom)
	if managedIsObject && customIsObject {
		return mergeSettingsDocs(managedDoc, customDoc)
	}
	if managedIsList || customIsList || managedIsObject || customIsObject {
		return custom, false
	}
	// Two scalars: there is nothing to keep from the managed one.
	return custom, true
}

// settingsValueObject reports whether v is a JSON object and returns
// it as an ordered document. Two spellings of an object meet here: the
// plain `map[string]any` a YAML hatch decodes to, and the
// `*OrderedJSON` the adapters that care about key order build their
// blocks with. A hook block is the worked example: qoder and augment
// both render `hooks` through one so the vendor's lifecycle event
// order survives a sync.
//
// Only the plain map read as an object before, so `x-qoder.hooks`
// against a generated hook block fell to the unmergeable branch: the
// hatch replaced the whole generated block and the note called two
// objects a shape conflict. Objects merge key by key, which is the
// rule the spec format documents, so the ordered document has to be
// seen as the object it is (#976).
//
// The plain map converts through its JSON form, so its keys land
// alphabetically, the order `encoding/json` would have given them on
// the way to the file anyway.
func settingsValueObject(v any) (*OrderedJSON, bool) {
	switch obj := v.(type) {
	case nil:
		return nil, false
	case *OrderedJSON:
		if obj == nil {
			return nil, false
		}
		return obj, true
	case map[string]any:
		raw, err := marshalSettingsValue(obj)
		if err != nil {
			return nil, false
		}
		doc := NewOrderedJSON()
		if err := doc.UnmarshalJSON(raw); err != nil {
			return nil, false
		}
		return doc, true
	}
	return nil, false
}

// mergeSettingsDocs merges two objects as ordered documents: every key
// the managed document holds keeps its position, a key both sides hold
// merges by the same rule one level down, and a key only the hatch
// holds is appended after them.
//
// That is what mergeSettingsMaps does for two plain maps, with the one
// guarantee a plain map cannot make. An ordered document promises the
// author's top-level key sequence survives a sync and that a subtree
// nobody touched is written back byte for byte, so a key only one side
// carries is copied raw rather than decoded and re-encoded.
func mergeSettingsDocs(managed, custom *OrderedJSON) (*OrderedJSON, bool) {
	out := NewOrderedJSON()
	clean := true
	for _, key := range managed.Keys() {
		managedRaw, _ := managed.Get(key)
		customRaw, held := custom.Get(key)
		if !held {
			out.SetRaw(key, managedRaw)
			continue
		}
		merged, ok := mergeSettingsRaw(managedRaw, customRaw)
		if !ok {
			clean = false
		}
		out.SetRaw(key, merged)
	}
	for _, key := range custom.Keys() {
		if _, held := managed.Get(key); held {
			continue
		}
		raw, _ := custom.Get(key)
		out.SetRaw(key, raw)
	}
	return out, clean
}

// mergeSettingsRaw decodes one key's two raw values, merges them by
// the same rule, and re-encodes. Bytes `encoding/json` cannot read,
// or a merge result it cannot write back, keep the hatch value and
// report the conflict: the same replacement the caller would have
// made anyway, said out loud rather than performed in silence.
func mergeSettingsRaw(managed, custom json.RawMessage) (json.RawMessage, bool) {
	managedValue, err := decodeSettingsRaw(managed)
	if err != nil {
		return custom, false
	}
	customValue, err := decodeSettingsRaw(custom)
	if err != nil {
		return custom, false
	}
	merged, clean := mergeSettingsValues(managedValue, customValue)
	raw, err := marshalSettingsValue(merged)
	if err != nil {
		return custom, false
	}
	return raw, clean
}

// decodeSettingsRaw reads one raw JSON value into the plain shapes the
// merge walks. Numbers decode as json.Number so a value that only
// passes through is written back in the spelling it arrived in,
// rather than through float64.
func decodeSettingsRaw(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// marshalSettingsValue renders one value as compact JSON with HTML
// escaping off, the same reason MarshalJSONIndent gives: these bytes
// end up in a settings file a CLI reads, where a shell command's `&&`
// must stay `&&`. Compact because OrderedJSON re-indents every raw
// value on the way out.
func marshalSettingsValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
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
