package emit

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func settingsEntry(name string, meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: name, Meta: meta}
}

func TestSettingsCustomKeys_ReturnsTheTargetBlock(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{
			"model": "portable-model",
			"x-factory": map[string]any{
				"sandbox": map[string]any{"enabled": true},
			},
			"x-kilo": map[string]any{"sandbox": map[string]any{"enabled": false}},
		}),
	}
	got := SettingsCustomKeys(entries, "factory")
	want := map[string]any{"sandbox": map[string]any{"enabled": true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_LastSpecWinsPerKey(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"sandbox":     map[string]any{"enabled": true},
			"disableSpin": true,
		}}),
		settingsEntry("project", map[string]any{"x-factory": map[string]any{
			"sandbox": map[string]any{"enabled": false},
		}}),
	}
	got := SettingsCustomKeys(entries, "factory")
	want := map[string]any{
		"sandbox":     map[string]any{"enabled": false},
		"disableSpin": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_SkipsExcludedKeys(t *testing.T) {
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-augment": map[string]any{
			"toolPermissions": []any{map[string]any{"toolName": "terminal"}},
			"shell":           "/bin/zsh",
		}}),
	}
	got := SettingsCustomKeys(entries, "augment", "toolPermissions")
	want := map[string]any{"shell": "/bin/zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SettingsCustomKeys = %#v, want %#v", got, want)
	}
}

func TestSettingsCustomKeys_NilWithoutABlock(t *testing.T) {
	entries := []spec.Entry{settingsEntry("base", map[string]any{"model": "m"})}
	if got := SettingsCustomKeys(entries, "factory"); got != nil {
		t.Errorf("SettingsCustomKeys = %#v, want nil", got)
	}
}

// A scalar has nothing to merge, so the hatch replaces it outright:
// an author writing the vendor's own spelling is stating what that
// value should be. Lists and objects merge instead, see below (#966).
func TestMergeSettingsCustomKeys_ReplacesAManagedScalar(t *testing.T) {
	keys := map[string]any{"model": "portable-model"}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"model":   "droid-core",
			"sandbox": map[string]any{"enabled": true},
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "factory")
	if keys["model"] != "droid-core" {
		t.Errorf("model = %#v, want the x-factory block to win", keys["model"])
	}
	if _, ok := keys["sandbox"]; !ok {
		t.Errorf("sandbox missing: %#v", keys)
	}
}

func TestMergeSettingsCustomKeys_LeavesManagedKeysAloneWithoutABlock(t *testing.T) {
	keys := map[string]any{"model": "portable-model"}
	MergeSettingsCustomKeys(keys, []spec.Entry{settingsEntry("base", nil)}, "factory")
	if len(keys) != 1 || keys["model"] != "portable-model" {
		t.Errorf("keys = %#v, want the managed map untouched", keys)
	}
}

// A hatch list joins the translated one instead of erasing it. The
// deny tier is the case that matters: an author adding one blocked
// command means also, not only (#966).
func TestMergeSettingsCustomKeys_UnionsListsWithTheManagedOnes(t *testing.T) {
	keys := map[string]any{"commandBlocklist": []string{"rm *", "curl"}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"commandBlocklist": []any{"author-only", "curl"},
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "factory")
	want := []any{"rm *", "curl", "author-only"}
	if !reflect.DeepEqual(keys["commandBlocklist"], want) {
		t.Errorf("commandBlocklist = %#v, want %#v", keys["commandBlocklist"], want)
	}
}

// Two objects merge key by key, recursively, so a hatch `deny` list
// leaves the translated `allow` list where it was.
func TestMergeSettingsCustomKeys_MergesObjectsKeyByKey(t *testing.T) {
	keys := map[string]any{"permissions": map[string]any{
		"allow": []any{"Read(**)"},
		"deny":  []any{"Bash(rm:*)"},
	}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-claude": map[string]any{
			"permissions": map[string]any{
				"deny":        []any{"AuthorOnly"},
				"defaultMode": "plan",
			},
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "claude")
	want := map[string]any{
		"allow":       []any{"Read(**)"},
		"deny":        []any{"Bash(rm:*)", "AuthorOnly"},
		"defaultMode": "plan",
	}
	if !reflect.DeepEqual(keys["permissions"], want) {
		t.Errorf("permissions = %#v, want %#v", keys["permissions"], want)
	}
}

// Two shapes that cannot merge keep the hatch value, the precedence a
// frontmatter override already has, and say so in a coverage note.
func TestMergeSettingsCustomKeys_NotesAShapeConflict(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	keys := map[string]any{"commandBlocklist": []string{"rm *"}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"commandBlocklist": "author-only",
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "factory")
	FlushCoverageNotes()

	if keys["commandBlocklist"] != "author-only" {
		t.Errorf("commandBlocklist = %#v, want the hatch value to win", keys["commandBlocklist"])
	}
	note := buf.String()
	for _, want := range []string{"commandBlocklist", "factory", "x-factory"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
}

// A clean merge stays silent. The note is for the shape conflict
// alone, so a plain union never adds a line to every sync.
func TestMergeSettingsCustomKeys_UnionRaisesNoNote(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	keys := map[string]any{"commandBlocklist": []string{"rm *"}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-factory": map[string]any{
			"commandBlocklist": []any{"author-only"},
		}}),
	}
	MergeSettingsCustomKeys(keys, entries, "factory")
	FlushCoverageNotes()

	if note := buf.String(); note != "" {
		t.Errorf("a clean union must stay silent, got: %s", note)
	}
}

// A server both sides name is taken from the hatch whole. Merging
// inside the record put the hatch's `command` beside the spec's
// `args`, which runs the author's binary with the previous binary's
// flags (#974).
func TestMergeSettingsCustomRecordMap_TakesACollidingRecordWhole(t *testing.T) {
	keys := map[string]any{"mcpServers": map[string]any{
		"foo": map[string]any{"command": "old-binary", "args": []any{"--from-spec"}},
	}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-qoder": map[string]any{
			"mcpServers": map[string]any{
				"foo": map[string]any{"command": "new-binary", "args": []any{"--from-hatch"}},
			},
		}}),
	}
	MergeSettingsCustomRecordMap(keys, entries, "qoder", "mcpServers")

	servers, ok := keys["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers = %#v, want a map", keys["mcpServers"])
	}
	foo, ok := servers["foo"].(map[string]any)
	if !ok {
		t.Fatalf("foo = %#v, want a map", servers["foo"])
	}
	if got := foo["command"]; got != "new-binary" {
		t.Errorf("command = %#v, want the hatch's", got)
	}
	args, _ := foo["args"].([]any)
	if len(args) != 1 || args[0] != "--from-hatch" {
		t.Errorf("args = %#v, want only the hatch's; the spec's must not ride along", foo["args"])
	}
}

// The two transports never end up in one entry. A `url` from the
// hatch replaces a stdio record rather than joining it (#974).
func TestMergeSettingsCustomRecordMap_ReplacesAStdioRecordWithAnHTTPOne(t *testing.T) {
	keys := map[string]any{"mcpServers": map[string]any{
		"foo": map[string]any{"command": "old-binary", "args": []any{"--from-spec"}},
	}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-qoder": map[string]any{
			"mcpServers": map[string]any{
				"foo": map[string]any{"url": "https://example.test/mcp"},
			},
		}}),
	}
	MergeSettingsCustomRecordMap(keys, entries, "qoder", "mcpServers")

	foo := keys["mcpServers"].(map[string]any)["foo"].(map[string]any)
	for _, stale := range []string{"command", "args"} {
		if _, held := foo[stale]; held {
			t.Errorf("%q survived onto an HTTP record: %#v", stale, foo)
		}
	}
	if foo["url"] != "https://example.test/mcp" {
		t.Errorf("url = %#v, want the hatch's", foo["url"])
	}
}

// Union at the registry level is the #966 fix and stays: a server
// only the specs named survives, and one only the author named lands.
func TestMergeSettingsCustomRecordMap_UnionsByName(t *testing.T) {
	keys := map[string]any{"mcpServers": map[string]any{
		"keepme": map[string]any{"command": "spec-only"},
	}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-qoder": map[string]any{
			"mcpServers": map[string]any{"hatchonly": map[string]any{"command": "author"}},
		}}),
	}
	MergeSettingsCustomRecordMap(keys, entries, "qoder", "mcpServers")

	servers := keys["mcpServers"].(map[string]any)
	for _, name := range []string{"keepme", "hatchonly"} {
		if _, held := servers[name]; !held {
			t.Errorf("%q missing from the union: %#v", name, servers)
		}
	}
}

// Nothing managed under the key means the hatch stands alone, with
// no note: there is no translated half for it to displace.
func TestMergeSettingsCustomRecordMap_TakesTheHatchWhenNothingIsManaged(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	keys := map[string]any{}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-qoder": map[string]any{
			"mcpServers": map[string]any{"only": map[string]any{"command": "author"}},
		}}),
	}
	MergeSettingsCustomRecordMap(keys, entries, "qoder", "mcpServers")
	FlushCoverageNotes()

	if _, held := keys["mcpServers"].(map[string]any)["only"]; !held {
		t.Errorf("mcpServers = %#v, want the hatch value", keys["mcpServers"])
	}
	if note := buf.String(); note != "" {
		t.Errorf("no managed half means no note, got: %s", note)
	}
}

// A hatch that is not a map of records cannot be unioned by name, so
// it replaces and says so rather than replacing in silence.
func TestMergeSettingsCustomRecordMap_NotesAShapeConflict(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	keys := map[string]any{"mcpServers": map[string]any{"foo": map[string]any{"command": "spec"}}}
	entries := []spec.Entry{
		settingsEntry("base", map[string]any{"x-qoder": map[string]any{
			"mcpServers": "not-a-map",
		}}),
	}
	MergeSettingsCustomRecordMap(keys, entries, "qoder", "mcpServers")
	FlushCoverageNotes()

	if keys["mcpServers"] != "not-a-map" {
		t.Errorf("mcpServers = %#v, want the hatch value to win", keys["mcpServers"])
	}
	note := buf.String()
	for _, want := range []string{"mcpServers", "qoder", "x-qoder"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
}
