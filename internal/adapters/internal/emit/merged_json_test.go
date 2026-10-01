package emit

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMergeJSONFileNested_RecordsTheKeysSyncSet(t *testing.T) {
	testutil.TempCwd(t)
	const path = "settings.json"
	if err := os.WriteFile(path, []byte(`{"model":{"temperature":0.2},"skills":{"urls":["u"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartDetailedRecording()
	err := sess.MergeJSONFileNested(path, map[string]any{
		"model":  map[string]any{"name": "new"},
		"hooks":  map[string]any{"AfterTool": []any{}},
		"kept":   CarriedJSONValue([]any{"user"}),
		"skills": map[string]any{"paths": CarriedJSONValue([]any{"mine", "ours"})},
	}, []string{"model", "skills"}, false)
	writes := sess.StopDetailedRecording()
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || !writes[0].Merged {
		t.Fatalf("want one merged write, got %#v", writes)
	}
	keys := slices.Clone(writes[0].Keys)
	slices.SortFunc(keys, slices.Compare[[]string])
	want := [][]string{{"hooks"}, {"model", "name"}}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestReleaseMergedJSON(t *testing.T) {
	const path = "settings.json"
	keys := [][]string{{"hooks"}, {"model", "name"}, {"env", "AGNOSTIC_AI_TARGET"}}
	for _, tc := range []struct {
		name    string
		before  string
		created bool
		want    MergedRelease
		after   string
	}{
		{"keeps user keys", `{"ui":1,"hooks":{},"model":{"name":"m","temperature":0.2},"env":{"AGNOSTIC_AI_TARGET":"x"}}`, true, MergedStripped, `{"ui":1,"model":{"temperature":0.2}}`},
		{"removes a file sync created", `{"hooks":{},"model":{"name":"m"}}`, true, MergedRemoved, ""},
		{"keeps an emptied user file", `{"hooks":{}}`, false, MergedStripped, `{}`},
		{"leaves a file without sync keys", `{"ui":1}`, true, MergedUnchanged, `{"ui":1}`},
		{"keeps a file it cannot parse", `{"hooks":`, true, MergedKept, `{"hooks":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			if err := os.WriteFile(path, []byte(tc.before), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := NewSession().ReleaseMergedJSON(path, keys, tc.created, false)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("result = %v, want %v", got, tc.want)
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
