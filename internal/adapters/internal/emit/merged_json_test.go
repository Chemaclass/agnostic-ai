package emit

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMergeJSONFileNested_RecordsTheValuesSyncSet(t *testing.T) {
	testutil.TempCwd(t)
	const path = "settings.json"
	if err := os.WriteFile(path, []byte(`{"model":{"temperature":0.2},"skills":{"urls":["u"]},"permissions":{"allow":["mine"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartDetailedRecording()
	err := sess.MergeJSONFileNested(path, map[string]any{
		"model":       map[string]any{"name": "new"},
		"hooks":       map[string]any{"AfterTool": []any{}},
		"kept":        CarriedJSONValue([]any{"user"}),
		"skills":      map[string]any{"paths": CarriedJSONValue([]any{"mine", "ours"})},
		"permissions": map[string]any{"allow": ClaimedJSONItems([]any{"mine", "ours"}, []string{"ours"}), "deny": ClaimedJSONItems([]any{"x"}, nil)},
	}, []string{"model", "skills", "permissions"}, false)
	writes := sess.StopDetailedRecording()
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || !writes[0].Merged {
		t.Fatalf("want one merged write, got %#v", writes)
	}
	keys := slices.Clone(writes[0].Keys)
	slices.SortFunc(keys, func(a, b MergedKey) int { return slices.Compare(a.Path, b.Path) })
	var paths [][]string
	for _, key := range keys {
		paths = append(paths, key.Path)
		if key.Items == nil && key.Sum == "" {
			t.Errorf("%v has no value sum", key.Path)
		}
	}
	want := [][]string{{"hooks"}, {"model", "name"}, {"permissions", "allow"}}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("keys = %v, want %v", paths, want)
	}
	if !reflect.DeepEqual(keys[2].Items, []string{ContentSum("ours")}) {
		t.Errorf("items = %v", keys[2].Items)
	}
	released := slices.Clone(writes[0].Released)
	slices.SortFunc(released, slices.Compare[[]string])
	if want := [][]string{{"kept"}, {"skills", "paths"}}; !reflect.DeepEqual(released, want) {
		t.Errorf("released = %v, want %v", released, want)
	}
}

func TestMergeJSONFileNested_ClaimsOnlyTheNamedObjectEntries(t *testing.T) {
	testutil.TempCwd(t)
	const path = "opencode.json"
	if err := os.WriteFile(path, []byte(`{"permission":{"bash":"ask","external_directory":{"mine/**":"allow"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartDetailedRecording()
	err := sess.MergeJSONFileNested(path, map[string]any{
		"permission": map[string]any{"external_directory": ClaimedJSONEntries(map[string]any{"mine/**": "allow", "ours/**": "allow"}, []string{"ours/**"})},
	}, []string{"permission"}, false)
	writes := sess.StopDetailedRecording()
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("want one write, got %#v", writes)
	}
	if want := []MergedKey{{Path: []string{"permission", "external_directory", "ours/**"}, Sum: jsonValueSum(json.RawMessage(`"allow"`))}}; !reflect.DeepEqual(writes[0].Keys, want) {
		t.Errorf("keys = %#v, want %#v", writes[0].Keys, want)
	}
	if want := [][]string{{"permission", "external_directory"}}; !reflect.DeepEqual(writes[0].Released, want) {
		t.Errorf("released = %v, want %v", writes[0].Released, want)
	}
	var doc map[string]map[string]any
	if err := json.Unmarshal([]byte(readFileString(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["permission"]["bash"] != "ask" || len(doc["permission"]["external_directory"].(map[string]any)) != 2 {
		t.Errorf("permission = %v", doc["permission"])
	}
}

func TestMergeJSONFileNested_KeepsTheObjectOrderOnDisk(t *testing.T) {
	testutil.TempCwd(t)
	const path = "opencode.json"
	if err := os.WriteFile(path, []byte(`{"permission":{"read":"allow","bash":{"*":"ask","git *":"allow","git push *":"deny","a":"allow"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ordered := NewOrderedJSON()
	for _, key := range []string{"/tmp/b/**", "*"} {
		if err := ordered.Set(key, "allow"); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewSession().MergeJSONFileNested(path, map[string]any{
		"permission": map[string]any{"external_directory": ordered},
	}, []string{"permission"}, false); err != nil {
		t.Fatal(err)
	}
	doc := NewOrderedJSON()
	if err := json.Unmarshal([]byte(readFileString(t, path)), doc); err != nil {
		t.Fatal(err)
	}
	permission, _ := doc.Get("permission")
	object := NewOrderedJSON()
	if err := json.Unmarshal(permission, object); err != nil {
		t.Fatal(err)
	}
	if want := []string{"read", "bash", "external_directory"}; !slices.Equal(object.Keys(), want) {
		t.Errorf("permission keys = %v, want %v", object.Keys(), want)
	}
	for key, want := range map[string][]string{"bash": {"*", "git *", "git push *", "a"}, "external_directory": {"/tmp/b/**", "*"}} {
		raw, _ := object.Get(key)
		child := NewOrderedJSON()
		if err := json.Unmarshal(raw, child); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(child.Keys(), want) {
			t.Errorf("%s keys = %v, want %v", key, child.Keys(), want)
		}
	}
}

// A key an earlier sync claimed whole that now merges child by child
// loses the whole claim. Its old value goes first while unchanged, as a
// stale claim's release would take it, and stays once edited.
func TestMergeJSONFileNested_MovesAWholeClaimToTheChildren(t *testing.T) {
	for _, tc := range []struct {
		name, onDisk string
		kept         bool
	}{
		{"edited", `{"permission":{"read":"allow","bash":"deny"}}`, true},
		{"unchanged", `{"permission":{"read":"allow"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			const path = "opencode.json"
			if err := os.WriteFile(path, []byte(tc.onDisk), 0o644); err != nil {
				t.Fatal(err)
			}
			PriorMergedKeys = func(string) []MergedKey {
				return []MergedKey{{Path: []string{"permission"}, Sum: jsonValueSum(json.RawMessage(`{"read":"allow"}`))}}
			}
			defer func() { PriorMergedKeys = nil }()
			sess := NewSession()
			if got := sess.ClaimsUnchangedValue(path, false, "permission"); got == tc.kept {
				t.Errorf("ClaimsUnchangedValue = %v", got)
			}
			sess.StartDetailedRecording()
			err := sess.MergeJSONFileNested(path, map[string]any{
				"permission": map[string]any{"external_directory": ClaimedJSONEntries(map[string]any{"s/**": "allow"}, []string{"s/**"})},
			}, []string{"permission"}, false)
			writes := sess.StopDetailedRecording()
			if err != nil {
				t.Fatal(err)
			}
			released := slices.Clone(writes[0].Released)
			slices.SortFunc(released, slices.Compare[[]string])
			if want := [][]string{{"permission"}, {"permission", "external_directory"}}; !reflect.DeepEqual(released, want) {
				t.Errorf("released = %v, want %v", released, want)
			}
			text := readFileString(t, path)
			if strings.Contains(text, `"read": "allow"`) != tc.kept || !strings.Contains(text, `"s/**": "allow"`) {
				t.Errorf("file = %s", text)
			}
		})
	}
}

func TestReleaseMergedJSON(t *testing.T) {
	const path = "settings.json"
	sum := func(v string) string { return jsonValueSum(json.RawMessage(v)) }
	keys := []MergedKey{
		{Path: []string{"hooks"}, Sum: sum(`{}`)},
		{Path: []string{"model", "name"}, Sum: sum(`"m"`)},
		{Path: []string{"env", "AGNOSTIC_AI_TARGET"}, Sum: sum(`"x"`)},
		{Path: []string{"permissions", "allow"}, Items: []string{ContentSum("ours")}},
	}
	for _, tc := range []struct {
		name    string
		before  string
		created bool
		force   bool
		want    MergedRelease
		after   string
	}{
		{"keeps user keys", `{"ui":1,"hooks":{},"model":{"name":"m","temperature":0.2},"env":{"AGNOSTIC_AI_TARGET":"x"},"permissions":{"allow":["mine","ours"]}}`, true, false, MergedStripped, `{"ui":1,"model":{"temperature":0.2},"permissions":{"allow":["mine"]}}`},
		{"removes a file sync created", `{"hooks":{},"model":{"name":"m"},"permissions":{"allow":["ours"]}}`, true, false, MergedRemoved, ""},
		{"keeps an emptied user file", `{"hooks":{}}`, false, false, MergedStripped, `{}`},
		{"leaves a file without sync keys", `{"ui":1}`, true, false, MergedUnchanged, `{"ui":1}`},
		{"keeps an edited value", `{"hooks":{"mine":[]},"model":{"name":"m"}}`, true, false, MergedEdited, `{"hooks":{"mine":[]}}`},
		{"force takes an edited value too", `{"hooks":{"mine":[]},"ui":1}`, true, true, MergedStripped, `{"ui":1}`},
		{"keeps a file it cannot parse", `{"hooks":`, true, false, MergedKept, `{"hooks":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			if err := os.WriteFile(path, []byte(tc.before), 0o644); err != nil {
				t.Fatal(err)
			}
			got, edited, err := NewSession().ReleaseMergedJSON(path, keys, tc.created, tc.force, false)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("result = %v, want %v", got, tc.want)
			}
			if (got == MergedEdited) != (len(edited) == 1 && slices.Equal(edited[0].Path, []string{"hooks"})) {
				t.Errorf("edited = %v", edited)
			}
			data, err := os.ReadFile(path)
			if tc.after == "" {
				if err == nil {
					t.Errorf("file still exists:\n%s", data)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == MergedKept {
				if string(data) != tc.after {
					t.Errorf("file changed:\n%s", data)
				}
				return
			}
			var gotDoc, wantDoc any
			if err := json.Unmarshal(data, &gotDoc); err != nil {
				t.Fatalf("%v\n%s", err, data)
			}
			if err := json.Unmarshal([]byte(tc.after), &wantDoc); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotDoc, wantDoc) {
				t.Errorf("file =\n%s\nwant %s", data, tc.after)
			}
		})
	}
}
