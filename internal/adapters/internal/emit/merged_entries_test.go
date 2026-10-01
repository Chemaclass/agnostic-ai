package emit

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMergeJSONEntries_UsesPriorClaims(t *testing.T) {
	const path = "settings.json"
	sum := func(v string) string { return jsonValueSum(json.RawMessage(v)) }
	for _, tc := range []struct {
		name   string
		before string
		prior  []MergedKey
		want   []string
	}{
		{
			"first sync keeps the user's entries",
			`{"mcpServers":{"mine":{"command":"m"}}}`,
			nil,
			[]string{"gh", "mine"},
		},
		{
			"an unchanged entry sync wrote leaves",
			`{"mcpServers":{"mine":{"command":"m"},"old":{"command":"o"}}}`,
			[]MergedKey{{Path: []string{"mcpServers", "old"}, Sum: sum(`{"command":"o"}`)}},
			[]string{"gh", "mine"},
		},
		{
			"an entry the user edited since stays",
			`{"mcpServers":{"old":{"command":"edited"}}}`,
			[]MergedKey{{Path: []string{"mcpServers", "old"}, Sum: sum(`{"command":"o"}`)}},
			[]string{"gh", "old"},
		},
		{
			"an unchanged map sync wrote whole is sync's",
			`{"mcpServers":{"old":{"command":"o"}}}`,
			[]MergedKey{{Path: []string{"mcpServers"}, Sum: sum(`{"old":{"command":"o"}}`)}},
			[]string{"gh"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			if err := os.WriteFile(path, []byte(tc.before), 0o644); err != nil {
				t.Fatal(err)
			}
			PriorMergedKeys = func(string) []MergedKey { return tc.prior }
			defer func() { PriorMergedKeys = nil }()
			sess := NewSession()
			sess.StartDetailedRecording()
			err := sess.MergeJSONFile(path, map[string]any{"mcpServers": MergeJSONEntries(map[string]any{"gh": map[string]any{"command": "npx"}})}, false)
			writes := sess.StopDetailedRecording()
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				MCPServers map[string]any `json:"mcpServers"`
			}
			data, _ := os.ReadFile(path)
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, name := range []string{"gh", "mine", "old"} {
				if _, ok := doc.MCPServers[name]; ok {
					names = append(names, name)
				}
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Errorf("servers = %v, want %v", names, tc.want)
			}
			if len(writes) != 1 || len(writes[0].Keys) != 1 || !reflect.DeepEqual(writes[0].Keys[0].Path, []string{"mcpServers", "gh"}) {
				t.Errorf("claims = %#v, want only mcpServers.gh", writes)
			}
		})
	}
}

// A server JSON cannot hold, such as one with a YAML .nan, fails the
// write and names the file, key, and server, leaving the file as it was
// (#1561).
func TestMergeJSONEntries_FailsOnAServerJSONCannotHold(t *testing.T) {
	const path = "settings.json"
	const before = `{"mcpServers":{"old":{"command":"o"}}}`
	testutil.TempCwd(t)
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	servers := map[string]any{"gh": map[string]any{"command": "npx", "timeout": math.NaN()}}
	err := NewSession().MergeJSONFile(path, map[string]any{"mcpServers": MergeJSONEntries(servers)}, false)
	if err == nil {
		t.Fatal("want an error for a server JSON cannot hold")
	}
	for _, want := range []string{path, "mcpServers", `"gh"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if got, _ := os.ReadFile(path); string(got) != before {
		t.Errorf("file = %s, want it unchanged", got)
	}
}
