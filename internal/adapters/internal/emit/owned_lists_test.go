package emit

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decodeLists(t *testing.T, raw string) map[string][]any {
	t.Helper()
	var out map[string][]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMergeOwnedLists_KeepsEntriesSyncDidNotWrite(t *testing.T) {
	disk := decodeLists(t, `{"PreToolUse": [{"matcher": "Bash", "command": "./guard.sh"}]}`)
	planned := decodeLists(t, `{"SessionStart": [{"command": "memory"}]}`)

	merged, claims := mergeOwnedLists(nil, false, disk, planned)

	want := decodeLists(t, `{"PreToolUse": [{"matcher": "Bash", "command": "./guard.sh"}], "SessionStart": [{"command": "memory"}]}`)
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}
	if len(claims["SessionStart"]) != 1 || len(claims["PreToolUse"]) != 0 {
		t.Errorf("claims = %v", claims)
	}
}

func TestMergeOwnedLists_ReplacesWhatSyncClaimedBefore(t *testing.T) {
	old := map[string]any{"command": "old"}
	disk := map[string][]any{"SessionStart": {old, map[string]any{"command": "mine"}}}
	planned := map[string][]any{"SessionStart": {map[string]any{"command": "new"}}}
	prior := map[string][]string{"SessionStart": {ContentSum(canonicalJSON(old))}}

	merged, _ := mergeOwnedLists(prior, false, disk, planned)

	want := map[string][]any{"SessionStart": {map[string]any{"command": "mine"}, map[string]any{"command": "new"}}}
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}
}

func TestMergeOwnedLists_AdoptsAnEqualEntryInsteadOfDoublingIt(t *testing.T) {
	entry := map[string]any{"command": "memory"}
	disk := map[string][]any{"SessionStart": {entry}}
	planned := map[string][]any{"SessionStart": {entry}}

	merged, claims := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["SessionStart"]) != 1 || len(claims["SessionStart"]) != 1 {
		t.Errorf("merged = %v, claims = %v", merged, claims)
	}
}

func TestMergeOwnedLists_DropsAnEventLeftEmpty(t *testing.T) {
	old := map[string]any{"command": "old"}
	disk := map[string][]any{"SessionStart": {old}}
	prior := map[string][]string{"SessionStart": {ContentSum(canonicalJSON(old))}}

	merged, _ := mergeOwnedLists(prior, false, disk, nil)

	if _, ok := merged["SessionStart"]; ok {
		t.Errorf("empty event kept: %v", merged)
	}
}

// An earlier version claimed the whole hook map; while it is unchanged,
// every entry in it is sync's.
func TestMergeOwnedLists_TreatsAnUnchangedWholeClaimAsSyncs(t *testing.T) {
	disk := map[string][]any{"SessionStart": {map[string]any{"command": "old"}}}
	planned := map[string][]any{"SessionStart": {map[string]any{"command": "new"}}}

	merged, _ := mergeOwnedLists(nil, true, disk, planned)

	want := map[string][]any{"SessionStart": {map[string]any{"command": "new"}}}
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged = %v, want %v", merged, want)
	}
}

func TestWithoutItems_DropsClaimedObjectEntries(t *testing.T) {
	entry := map[string]any{"command": "memory"}
	raw, _ := json.Marshal([]any{entry, map[string]any{"command": "mine"}})

	value, keep, changed := withoutItems(raw, []string{ContentSum(canonicalJSON(entry))})

	if !changed || !keep || len(value.([]json.RawMessage)) != 1 {
		t.Errorf("value %v keep %v changed %v", value, keep, changed)
	}
}

// import turns a hand-written hook into a spec but leaves the entry on
// disk; sync renders it with its own extras and must not run it twice.
func TestMergeOwnedLists_AdoptsAnImportedEntryInItsOriginalShape(t *testing.T) {
	disk := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"./guard.sh"}]}`)}}
	planned := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; ./guard.sh","commandWindows":"./guard.sh"}]}`)}}

	merged, _ := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["PreToolUse"]) != 1 {
		t.Errorf("hook written twice: %v", merged)
	}
}

func TestMergeOwnedLists_KeepsAUserHookThatDiffersOnlyInArgs(t *testing.T) {
	disk := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"./guard","args":["--deny"]}]}`)}}
	planned := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"./guard","args":["--audit"]}]}`)}}

	merged, _ := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["PreToolUse"]) != 2 {
		t.Errorf("distinct user hook dropped: %v", merged)
	}
}

// import turns two groups that share a matcher into specs that sync
// renders as one group; neither original group stays.
func TestMergeOwnedLists_AdoptsImportedGroupsSyncConsolidates(t *testing.T) {
	disk := map[string][]any{"PreToolUse": {
		json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"./a.sh"}]}`),
		json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"./b.sh"}]}`),
	}}
	planned := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; ./a.sh"},{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; ./b.sh"}]}`)}}

	merged, _ := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["PreToolUse"]) != 1 {
		t.Errorf("imported groups kept beside sync's: %v", merged)
	}
}

func TestMergeOwnedLists_KeepsAUserHookWithItsOwnWindowsCommand(t *testing.T) {
	disk := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"lint.sh","commandWindows":"guard.ps1"}]}`)}}
	planned := map[string][]any{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; lint.sh","commandWindows":"lint.sh"}]}`)}}

	merged, _ := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["PreToolUse"]) != 2 {
		t.Errorf("user's Windows command dropped: %v", merged)
	}
}

func TestMergeOwnedLists_KeepsASequentialGroupSyncRendersApart(t *testing.T) {
	disk := map[string][]any{"BeforeTool": {json.RawMessage(`{"matcher":"run_shell_command","sequential":true,"hooks":[{"type":"command","command":"prepare.sh"},{"type":"command","command":"consume.sh"}]}`)}}
	planned := map[string][]any{"BeforeTool": {
		json.RawMessage(`{"matcher":"run_shell_command","hooks":[{"type":"command","command":"prepare.sh"}]}`),
		json.RawMessage(`{"matcher":"run_shell_command","hooks":[{"type":"command","command":"consume.sh"}]}`),
	}}

	merged, _ := mergeOwnedLists(nil, false, disk, planned)

	if len(merged["BeforeTool"]) != 3 {
		t.Errorf("sequential user group dropped: %v", merged)
	}
}

func TestWithoutUserItems_LeavesTheUserEntryAndKeepsSyncsOwn(t *testing.T) {
	PriorMergedKeys = func(string) []MergedKey {
		return []MergedKey{{Path: []string{"permissions", "allow"}, Items: []string{ContentSum("Write(/ours/**)")}}}
	}
	defer func() { PriorMergedKeys = nil }()
	onDisk := []any{"Write(/mine/**)", "Write(/ours/**)"}
	planned := []any{"Write(/mine/**)", "Write(/ours/**)", "Write(/new/**)"}
	got := WithoutUserItems("config.json", []string{"permissions", "allow"}, onDisk, planned)
	if want := []any{"Write(/ours/**)", "Write(/new/**)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("planned = %v, want %v", got, want)
	}
}
