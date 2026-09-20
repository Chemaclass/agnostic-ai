package emit

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// A hook block is an *OrderedJSON, not a plain map, so the shape
// comparison had to learn to see it as the object it is. Before that,
// `x-<target>.hooks` against a generated block took the unmergeable
// branch: the whole generated block was replaced, and the note called
// two objects a shape conflict (#976).
func TestMergeSettingsCustomValue_MergesAnOrderedDocument(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	managed := NewOrderedJSON()
	if err := managed.Set("PreToolUse", []any{map[string]any{"matcher": "Bash"}}); err != nil {
		t.Fatal(err)
	}
	if err := managed.Set("SessionStart", []any{map[string]any{"matcher": "translated"}}); err != nil {
		t.Fatal(err)
	}
	custom := map[string]any{
		"SessionStart": []any{map[string]any{"matcher": "authored"}},
		"Stop":         []any{map[string]any{"matcher": "authored"}},
	}

	merged := MergeSettingsCustomValue("qoder", "hooks", managed, custom)
	FlushCoverageNotes()

	doc, ok := merged.(*OrderedJSON)
	if !ok {
		t.Fatalf("merged = %T, want an ordered document", merged)
	}
	// Managed keys hold their positions; a hatch-only key lands after.
	if want := []string{"PreToolUse", "SessionStart", "Stop"}; !reflect.DeepEqual(doc.Keys(), want) {
		t.Errorf("keys = %#v, want %#v", doc.Keys(), want)
	}
	raw, _ := doc.Get("SessionStart")
	for _, want := range []string{"translated", "authored"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("SessionStart = %s, want both matchers unioned", raw)
		}
	}
	if note := buf.String(); note != "" {
		t.Errorf("two objects are not a shape conflict, got: %s", note)
	}
}

// The merge writes settings files a CLI reads, not a browser, so a
// shell command's `&&` has to survive the ordered-document path the
// same way MarshalJSONIndent keeps it on every other path.
func TestMergeSettingsCustomValue_OrderedDocumentKeepsShellOperators(t *testing.T) {
	managed := NewOrderedJSON()
	if err := managed.Set("Stop", map[string]any{"keep": true}); err != nil {
		t.Fatal(err)
	}
	merged := MergeSettingsCustomValue("qoder", "hooks", managed,
		map[string]any{"Stop": map[string]any{"command": "go build && go test"}})

	raw, err := MarshalJSONIndent(merged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "go build && go test") {
		t.Errorf("shell operators were escaped: %s", raw)
	}
	if !strings.Contains(string(raw), `"keep": true`) {
		t.Errorf("the managed sibling was dropped: %s", raw)
	}
}

// An ordered document against a list is still a real conflict, and
// still says so.
func TestMergeSettingsCustomValue_OrderedDocumentAgainstAListStillNotes(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	managed := NewOrderedJSON()
	if err := managed.Set("Stop", []any{"a"}); err != nil {
		t.Fatal(err)
	}
	merged := MergeSettingsCustomValue("qoder", "hooks", managed, []any{"b"})
	FlushCoverageNotes()

	if !reflect.DeepEqual(merged, []any{"b"}) {
		t.Errorf("merged = %#v, want the hatch value", merged)
	}
	if note := buf.String(); !strings.Contains(note, "x-qoder.hooks") {
		t.Errorf("expected a shape-conflict note, got: %s", note)
	}
}

// A hatch of the wrong shape under a key the adapter reads itself used
// to read as absent, so the author's policy did nothing and nothing
// said so. It still cannot be honored, but it is no longer silent.
func TestSettingsCustomObject_NotesAWrongShapedValue(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	entry := settingsEntry("base", map[string]any{
		"x-kilo": map[string]any{"permission": []any{"bash"}},
	})
	if value, ok := SettingsCustomObject(entry, "kilo", "permission"); ok || value != nil {
		t.Errorf("SettingsCustomObject = %#v, %v, want nil, false", value, ok)
	}
	FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"x-kilo.permission", "kilo", "a list", "an object"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
}

func TestSettingsCustomList_NotesAWrongShapedValue(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	entry := settingsEntry("base", map[string]any{
		"x-augment": map[string]any{"toolPermissions": map[string]any{"toolName": "terminal"}},
	})
	if value := SettingsCustomList(entry, "augment", "toolPermissions"); value != nil {
		t.Errorf("SettingsCustomList = %#v, want nil", value)
	}
	FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"x-augment.toolPermissions", "augment", "an object", "a list"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected the note to mention %q, got: %s", want, note)
		}
	}
}

// An absent key, and a key written with no value, are both silence. A
// commented-out hatch is not a mistake to report.
func TestSettingsCustomAccessors_StayQuietWhenTheKeyIsAbsent(t *testing.T) {
	var buf bytes.Buffer
	restore := Warner
	Warner = &buf
	defer func() { Warner = restore }()
	ResetCoverageNotes()

	entries := []spec.Entry{
		settingsEntry("none", map[string]any{"x-kilo": map[string]any{"sandbox": true}}),
		settingsEntry("empty", map[string]any{"x-kilo": map[string]any{"permission": nil}}),
		settingsEntry("noblock", map[string]any{"model": "m"}),
	}
	for _, entry := range entries {
		if _, ok := SettingsCustomObject(entry, "kilo", "permission"); ok {
			t.Errorf("%s: want the hatch to read as absent", entry.Name)
		}
		if got := SettingsCustomList(entry, "kilo", "permission"); got != nil {
			t.Errorf("%s: want the hatch to read as absent, got %#v", entry.Name, got)
		}
	}
	FlushCoverageNotes()
	if note := buf.String(); note != "" {
		t.Errorf("an absent hatch must not note, got: %s", note)
	}
}
