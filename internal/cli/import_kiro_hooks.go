package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// kiroHooksDir holds one standalone JSON file per hook definition:
// "Each hook file is a standalone JSON file at `.kiro/hooks/<id>.json`"
// (kiro.dev/docs/hooks/, re-verified 2026-09-20). Filenames are
// free-form ("Any `.json` filename works"), so nothing about a file's
// name identifies the hooks inside it.
const kiroHooksDir = ".kiro/hooks"

// kiroHookOwnedKeys are the entry keys this importer reads itself. Any
// other key rides through under `x-kiro`: the vendor publishes no closed
// schema for this file (unlike Crush's `additionalProperties: false`),
// so an unrecognized key is future vendor surface, not a typo to drop.
// `confirm` reaches the spec this way, mirroring the emit side, where
// `x-kiro.confirm` flattens onto the entry.
var kiroHookOwnedKeys = []string{"name", "trigger", "matcher", "action", "timeout", "enabled", "description"}

// kiroHookEntry is one decoded object from a file's `hooks` array,
// split into the fields the portable spec carries and the `native`
// remainder destined for `x-kiro`.
type kiroHookEntry struct {
	name        string
	trigger     string
	matcher     string
	description string
	timeout     int
	hasTimeout  bool
	disabled    bool
	// command is set only for a plain `{"type": "command", "command":
	// ...}` action, the one shape the portable `command:` field carries.
	// Anything else lands in native["action"].
	command string
	native  map[string]any
}

// importKiroHooks reads every `.kiro/hooks/*.json` file and writes hook
// specs into dstDir. No-op when the directory is absent.
//
// Entries in one file sharing every field but `action.command`
// recombine into a single spec whose `command:` is a list, reversing
// the emit side's one-entry-per-command split (see the kiro adapter's
// buildHookEntries). Without that, a round-trip of one spec carrying
// three commands would return three specs. Entries differing in
// `trigger` never merge: a spec holds exactly one `event`.
//
// The `version` envelope is read past. The vendor pegs it at `"v1"` and
// the emit side writes that constant, so a spec field would have
// nothing to vary.
func importKiroHooks(root, dstDir string) (int, error) {
	dir := filepath.Join(root, filepath.FromSlash(kiroHooksDir))
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}
	used := map[string]bool{}
	count := 0
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		n, err := importKiroHookFile(filepath.Join(dir, f.Name()), dstDir, used)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

// importKiroHookFile reads one hook file, groups its entries, and
// writes one spec per group. used carries the spec names already taken
// by earlier files so a name declared twice keeps both hooks.
func importKiroHookFile(path, dstDir string, used map[string]bool) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var doc struct {
		Hooks []map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	count := 0
	for _, group := range groupKiroHookEntries(doc.Hooks, path) {
		written, err := writeKiroHookSpec(dstDir, group, used)
		if err != nil {
			return count, err
		}
		if written {
			count++
		}
	}
	return count, nil
}

// groupKiroHookEntries collapses a file's entries into groups that
// differ only in `action.command`, preserving first-seen order. The
// group key spans every field but the name and the command, so entries
// disagreeing on timeout, enabled state, or a native passthrough stay
// apart: merging those would silently move a value onto a command that
// never carried it.
//
// The name comes from the group's first entry, which drops the `-2`,
// `-3`, ... suffixes the emit side appends to keep entries in one file
// unique.
func groupKiroHookEntries(raw []map[string]any, path string) [][]kiroHookEntry {
	var order []string
	groups := map[string][]kiroHookEntry{}
	for _, item := range raw {
		e, ok := readKiroHookEntry(item, path)
		if !ok {
			continue
		}
		key := kiroHookGroupKey(e)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], e)
	}
	out := make([][]kiroHookEntry, 0, len(order))
	for _, key := range order {
		out = append(out, groups[key])
	}
	return out
}

// kiroHookGroupKey canonicalizes everything about an entry except its
// name and its command. A marshal failure yields an empty native
// segment, which can only over-group entries whose passthrough is not
// JSON-representable; nothing decoded from JSON can reach that state.
func kiroHookGroupKey(e kiroHookEntry) string {
	native, _ := json.Marshal(e.native)
	portable := e.command != ""
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%t\x00%t\x00%t\x00%s",
		e.trigger, e.matcher, e.description, e.timeout, e.hasTimeout, e.disabled, portable, native)
}

// readKiroHookEntry splits one decoded hook object into portable and
// native halves. Reports false, after a warning naming the entry, for
// the shapes the vendor table marks required or conditional and this
// file leaves unset: writing a spec from one of those would fail the
// next sync instead of the import the user is watching.
func readKiroHookEntry(raw map[string]any, path string) (kiroHookEntry, bool) {
	e := kiroHookEntry{
		name:   strings.TrimSpace(stringField(raw, "name")),
		native: map[string]any{},
	}
	label := e.name
	if label == "" {
		label = "(unnamed)"
	}
	e.trigger = strings.TrimSpace(stringField(raw, "trigger"))
	if e.trigger == "" {
		warnKiroHookSkipped(path, label, "no trigger")
		return kiroHookEntry{}, false
	}
	e.matcher = stringField(raw, "matcher")
	e.description = stringField(raw, "description")
	e.timeout, e.hasTimeout = kiroHookTimeout(raw["timeout"])
	// The vendor default is `true` and the emit side writes no key for
	// it, so only an explicit `false` carries information.
	if enabled, ok := raw["enabled"].(bool); ok && !enabled {
		e.disabled = true
	}
	action, ok := raw["action"].(map[string]any)
	if !ok {
		warnKiroHookSkipped(path, label, "no action object")
		return kiroHookEntry{}, false
	}
	if !readKiroHookAction(&e, action, path, label) {
		return kiroHookEntry{}, false
	}
	for k, v := range raw {
		if isKiroHookOwnedKey(k) {
			continue
		}
		e.native[k] = v
	}
	return e, true
}

// readKiroHookAction places the action on e, either as a portable
// command or under `x-kiro.action`.
//
// An agent action stays native on purpose. spec-format.md scopes the
// portable `type: prompt` handler to Claude Code, Cursor and Copilot;
// Kiro is not on that list, and `x-kiro.action` is the key the emit
// side already reads, so the action round-trips byte-identically. A
// command action carrying keys beyond `type` and `command` goes native
// too, since the portable field has nowhere to keep them.
func readKiroHookAction(e *kiroHookEntry, action map[string]any, path, label string) bool {
	switch stringField(action, "type") {
	case "command":
		command := stringField(action, "command")
		if strings.TrimSpace(command) == "" {
			warnKiroHookSkipped(path, label, `action.type "command" with no command`)
			return false
		}
		if len(action) > 2 {
			e.native["action"] = action
			return true
		}
		e.command = command
		return true
	case "agent":
		if strings.TrimSpace(stringField(action, "prompt")) == "" {
			warnKiroHookSkipped(path, label, `action.type "agent" with no prompt`)
			return false
		}
		e.native["action"] = action
		return true
	default:
		warnKiroHookSkipped(path, label, "action.type is neither command nor agent")
		return false
	}
}

// writeKiroHookSpec renders one group as a single hook spec. Reports
// false when the group duplicates a spec an earlier file already wrote.
func writeKiroHookSpec(dstDir string, group []kiroHookEntry, used map[string]bool) (bool, error) {
	first := group[0]
	var commands []string
	for _, e := range group {
		if e.command != "" {
			commands = append(commands, e.command)
		}
	}
	specName, fileName, ok := resolveKiroHookName(first, commands, used)
	if !ok {
		return false, nil
	}
	doc := map[string]any{
		"name":   specName,
		"event":  first.trigger,
		"target": "kiro",
	}
	switch {
	case len(commands) == 1:
		doc["command"] = commands[0]
	case len(commands) > 1:
		doc["command"] = commands
	}
	if first.matcher != "" {
		doc["matcher"] = first.matcher
	}
	if first.description != "" {
		doc["description"] = first.description
	}
	if first.hasTimeout {
		doc["timeout"] = first.timeout
	}
	if first.disabled {
		doc["disabled"] = true
	}
	if len(first.native) > 0 {
		doc["x-kiro"] = first.native
	}
	if err := writeHookSpecFile(dstDir, fileName, doc); err != nil {
		return false, err
	}
	return true, nil
}

// resolveKiroHookName picks the spec name and the filename for one
// group and records the filename in used.
//
// A Kiro name is a human-readable label, not an identifier (the
// vendor's own example is "Lint on save"), so the filename slugs while
// the label stays on `name:`. A label that cannot be a spec name at
// all, and a label a previous file already claimed, both fall back to
// the deterministic hookSpecName every other hook importer uses: two
// files may declare the same hook name, and a spec name is unique
// across the bundle, so one of the two has to be renamed for both
// hooks to survive.
func resolveKiroHookName(e kiroHookEntry, commands []string, used map[string]bool) (specName, fileName string, ok bool) {
	fallback := hookSpecName(e.trigger, e.matcher, kiroHookNameSeed(e, commands))
	specName = e.name
	if specName == "" || spec.ValidateName(spec.KindHook, specName) != nil {
		specName = fallback
	}
	fileName = hookMatcherSlug(specName)
	if fileName == "" {
		fileName = fallback
	}
	if used[fileName] {
		summaryf("  ! two kiro hook files declare the hook %q; importing the second as %q\n", e.name, fallback)
		specName, fileName = fallback, fallback
	}
	if used[fileName] {
		return "", "", false
	}
	used[fileName] = true
	return specName, fileName, true
}

// kiroHookNameSeed returns the content hookSpecName hashes over. A
// native action has no command list, so its canonical JSON stands in:
// two agent hooks sharing a trigger and matcher must not collapse onto
// one filename.
func kiroHookNameSeed(e kiroHookEntry, commands []string) []string {
	if len(commands) > 0 {
		return commands
	}
	raw, err := json.Marshal(e.native)
	if err != nil {
		return []string{e.name}
	}
	return []string{string(raw)}
}

// kiroHookTimeout reads a `timeout` value and reports whether the key
// was present. An explicit `0` disables the timeout ("`0` disables the
// timeout"), so it must stay distinguishable from an absent key, which
// keeps the vendor's 60-second default.
func kiroHookTimeout(raw any) (int, bool) {
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func isKiroHookOwnedKey(key string) bool {
	for _, k := range kiroHookOwnedKeys {
		if k == key {
			return true
		}
	}
	return false
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func warnKiroHookSkipped(path, label, reason string) {
	summaryf("  ! skipping kiro hook %q in %s: %s\n", label, path, reason)
}
