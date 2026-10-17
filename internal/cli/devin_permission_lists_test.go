package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	devinConfig     = ".devin/config.json"
	devinPolicySpec = ".agnostic-ai/settings/policy.yaml"
	devinPolicy     = "permissions:\n  allow: [\"Read(src/**)\"]\n  deny: [\"Bash(rm:*)\"]\n  ask: [\"Bash(git push:*)\"]\n"
)

func devinPermissionProject(t *testing.T, handWritten string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf]\n")
	if handWritten != "" {
		mustWriteFile(t, devinConfig, handWritten+"\n")
	}
	mustWriteFile(t, devinPolicySpec, devinPolicy)
}

func devinPermissionLists(t *testing.T) map[string]any {
	t.Helper()
	got, _ := readJSONMap(t, devinConfig)["permissions"].(map[string]any)
	out := map[string]any{}
	for list, value := range got {
		out[list] = value
	}
	return out
}

func devinHandEdit(t *testing.T, list, rule string) {
	t.Helper()
	doc := readJSONMap(t, devinConfig)
	permissions := doc["permissions"].(map[string]any)
	existing, _ := permissions[list].([]any)
	permissions[list] = append(existing, rule)
	writeJSONFile(t, devinConfig, doc)
}

// Sync owns the entries its specs produce in each rule list, not the
// list (#2184).
func TestSync_DevinPermissionListsKeepEntriesTheSpecsDoNotWrite(t *testing.T) {
	devinPermissionProject(t, `{"permissions": {"allow": ["Exec(ls)"], "deny": ["Exec(curl)"], "ask": ["Exec(make)"]}}`)
	runSyncOK(t)
	want := map[string]any{
		"allow": []any{"Exec(ls)", "Read(src/**)"},
		"deny":  []any{"Exec(curl)", "Exec(rm)"},
		"ask":   []any{"Exec(make)", "Exec(git push)"},
	}
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permissions = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")
	runSyncOK(t)
	runSyncOK(t, "--check")

	mustWriteFile(t, devinPolicySpec, "permissions:\n  deny: [\"Bash(sudo:*)\"]\n")
	runSyncOK(t)
	want = map[string]any{
		"allow": []any{"Exec(ls)"},
		"deny":  []any{"Exec(curl)", "Exec(sudo)"},
		"ask":   []any{"Exec(make)"},
	}
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permissions after a spec change = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")

	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	want = map[string]any{"allow": []any{"Exec(ls)"}, "deny": []any{"Exec(curl)"}, "ask": []any{"Exec(make)"}}
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, want) {
		t.Errorf("permissions after the spec left = %#v, want the hand-written entries", got)
	}
	runSyncOK(t, "--check")
}

// An entry the user adds after a sync stays through later syncs, and the
// spec's entry still leaves with its spec.
func TestSync_DevinPermissionListsKeepAnEntryAddedAfterSync(t *testing.T) {
	devinPermissionProject(t, "")
	runSyncOK(t)
	for list, rule := range map[string]string{"allow": "Exec(ls)", "deny": "Exec(curl)", "ask": "Exec(make)"} {
		devinHandEdit(t, list, rule)
	}
	runSyncOK(t)
	runSyncOK(t, "--check")
	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	want := map[string]any{"allow": []any{"Exec(ls)"}, "deny": []any{"Exec(curl)"}, "ask": []any{"Exec(make)"}}
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, want) {
		t.Errorf("permissions = %#v, want only the hand-written entries", got)
	}
	runSyncOK(t, "--check")
}

// A rule the user wrote that a spec also produces stays theirs.
func TestSync_DevinPermissionListsKeepARuleTheUserWroteFirst(t *testing.T) {
	devinPermissionProject(t, `{"permissions": {"deny": ["Exec(rm)"]}}`)
	runSyncOK(t)
	runSyncOK(t, "--check")
	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{"deny": []any{"Exec(rm)"}}) {
		t.Errorf("permissions = %#v, want the user's deny", got)
	}
	runSyncOK(t, "--check")
}

// asWholeListClaims rewrites the ledger the way a sync that claimed each
// permissions list whole left it.
func asWholeListClaims(t *testing.T) {
	t.Helper()
	path := filepath.Join(".agnostic-ai", ".sync-state")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	record := state["merged"].(map[string]any)[devinConfig].(map[string]any)
	var keys []any
	for _, key := range record["keys"].([]any) {
		if p := key.(map[string]any)["path"].([]any); p[0] != "permissions" {
			keys = append(keys, key)
		}
	}
	for list, value := range devinPermissionLists(t) {
		canonical, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, map[string]any{"path": []any{"permissions", list}, "sum": adapters.ContentSum(string(canonical))})
	}
	record["keys"] = keys
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, string(out)+"\n")
}

// A ledger written when sync claimed each list whole moves to one claim
// per entry, so the spec's entries still leave and the user's stay.
func TestSync_DevinPermissionListsUpgradeAWholeListClaim(t *testing.T) {
	t.Run("unchanged lists", func(t *testing.T) {
		devinPermissionProject(t, "")
		runSyncOK(t)
		asWholeListClaims(t)
		runSyncOK(t)
		runSyncOK(t, "--check")
		devinHandEdit(t, "deny", "Exec(curl)")
		removeSpecs(t, devinPolicySpec)
		runSyncOK(t)
		if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{"deny": []any{"Exec(curl)"}}) {
			t.Errorf("permissions = %#v, want only the hand-written deny", got)
		}
		runSyncOK(t, "--check")
	})
	t.Run("entry added since", func(t *testing.T) {
		devinPermissionProject(t, "")
		runSyncOK(t)
		asWholeListClaims(t)
		devinHandEdit(t, "deny", "Exec(curl)")
		mustWriteFile(t, devinPolicySpec, "permissions:\n  deny: [\"Bash(rm:*)\", \"Bash(sudo:*)\"]\n")
		runSyncOK(t)
		if got := devinPermissionLists(t)["deny"]; !reflect.DeepEqual(got, []any{"Exec(curl)", "Exec(rm)", "Exec(sudo)"}) {
			t.Fatalf("deny = %#v, want the hand entry before the spec's", got)
		}
		runSyncOK(t, "--check")
		removeSpecs(t, devinPolicySpec)
		runSyncOK(t)
		if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{"deny": []any{"Exec(curl)"}}) {
			t.Errorf("permissions = %#v, want only the hand-written deny", got)
		}
		runSyncOK(t, "--check")
	})
	t.Run("entry added before the first sync here", func(t *testing.T) {
		for _, list := range []string{"allow", "deny", "ask"} {
			devinPermissionProject(t, "")
			runSyncOK(t)
			asWholeListClaims(t)
			devinHandEdit(t, list, "Exec(curl)")
			runSyncOK(t)
			runSyncOK(t, "--check")
			removeSpecs(t, devinPolicySpec)
			runSyncOK(t)
			if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{list: []any{"Exec(curl)"}}) {
				t.Errorf("%s: permissions = %#v, want only the hand-written entry", list, got)
			}
			runSyncOK(t, "--check")
		}
	})
}

// A copy of a rule the user adds beside the one sync wrote stays theirs.
func TestSync_DevinPermissionListsKeepAnExtraCopyOfASpecRule(t *testing.T) {
	devinPermissionProject(t, "")
	runSyncOK(t)
	devinHandEdit(t, "deny", "Exec(rm)")
	runSyncOK(t)
	runSyncOK(t, "--check")
	if got := devinPermissionLists(t)["deny"]; !reflect.DeepEqual(got, []any{"Exec(rm)", "Exec(rm)"}) {
		t.Fatalf("deny = %#v, want both copies", got)
	}
	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{"deny": []any{"Exec(rm)"}}) {
		t.Errorf("permissions = %#v, want the user's copy", got)
	}
	runSyncOK(t, "--check")
}

// A rule that left a list stops being sync's, so one the user writes
// there later stays while a spec is present.
func TestSync_DevinPermissionListsReleaseARuleThatLeftBeforeTheUserWroteIt(t *testing.T) {
	devinPermissionProject(t, `{"permissions": {"deny": ["Exec(curl)"]}}`)
	mustWriteFile(t, devinPolicySpec, "permissions:\n  deny: [\"Bash(rm:*)\"]\n")
	runSyncOK(t)
	mustWriteFile(t, devinPolicySpec, "permissions:\n  deny: [\"Bash(curl:*)\"]\n")
	runSyncOK(t)
	runSyncOK(t, "--check")
	devinHandEdit(t, "deny", "Exec(rm)")
	runSyncOK(t)
	runSyncOK(t, "--check")
	if got := devinPermissionLists(t)["deny"]; !reflect.DeepEqual(got, []any{"Exec(curl)", "Exec(rm)"}) {
		t.Errorf("deny = %#v, want the hand-added rule kept", got)
	}
}

// A spec whose rules the file already holds adds nothing, so sync
// records no claim and a check right after the sync stays clean.
func TestSync_DevinPermissionListsClaimNothingWhenTheUserHoldsEveryRule(t *testing.T) {
	devinPermissionProject(t, `{"permissions": {"allow": ["Read(src/**)"]}}`)
	mustWriteFile(t, devinPolicySpec, "permissions:\n  allow: [\"Read(src/**)\"]\n")
	runSyncOK(t)
	runSyncOK(t, "--check")
	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	runSyncOK(t, "--check")
	if got := devinPermissionLists(t); !reflect.DeepEqual(got, map[string]any{"allow": []any{"Read(src/**)"}}) {
		t.Errorf("permissions = %#v, want the user's allow", got)
	}
}

// The copy sync wrote is the last one in the file, so a copy the user
// adds before it stays theirs and the file settles in one sync.
func TestSync_DevinPermissionListsKeepACopyAddedBeforeTheSpecRule(t *testing.T) {
	devinPermissionProject(t, "")
	mustWriteFile(t, devinPolicySpec, "permissions:\n  deny: [\"Bash(rm:*)\"]\n")
	runSyncOK(t)
	doc := readJSONMap(t, devinConfig)
	doc["permissions"].(map[string]any)["deny"] = []any{"Exec(rm)", "Exec(curl)", "Exec(rm)"}
	writeJSONFile(t, devinConfig, doc)
	runSyncOK(t)
	first := devinPermissionLists(t)["deny"]
	runSyncOK(t, "--check")
	runSyncOK(t)
	if got := devinPermissionLists(t)["deny"]; !reflect.DeepEqual(got, first) {
		t.Errorf("deny changed between syncs: %#v then %#v", first, got)
	}
	runSyncOK(t, "--check")
	if !reflect.DeepEqual(first, []any{"Exec(rm)", "Exec(curl)", "Exec(rm)"}) {
		t.Fatalf("deny = %#v, want the user's copy first and sync's last", first)
	}
	removeSpecs(t, devinPolicySpec)
	runSyncOK(t)
	if got := devinPermissionLists(t)["deny"]; !reflect.DeepEqual(got, []any{"Exec(rm)", "Exec(curl)"}) {
		t.Errorf("deny after the spec left = %#v, want the user's entries", got)
	}
	runSyncOK(t, "--check")
}
