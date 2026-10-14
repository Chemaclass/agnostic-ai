package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const openCodePermissionSpec = ".agnostic-ai/settings/policy.yaml"

func openCodePermissionProject(t *testing.T, handWritten string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	if handWritten != "" {
		mustWriteFile(t, "opencode.json", handWritten+"\n")
	}
	mustWriteFile(t, openCodePermissionSpec, "permissions:\n  allow: [Read]\n  deny: [\"Bash(rm:*)\"]\n")
}

func openCodePermission(t *testing.T) map[string]any {
	t.Helper()
	got, _ := readJSONMap(t, "opencode.json")["permission"].(map[string]any)
	return got
}

// Sync owns the permission keys its specs produce and nothing else in
// the object (#2175).
func TestSync_OpenCodePermissionKeepsKeysTheSpecsDoNotWrite(t *testing.T) {
	openCodePermissionProject(t, `{"permission": {"edit": "ask"}}`)
	runSyncOK(t)
	want := map[string]any{"edit": "ask", "read": "allow", "bash": map[string]any{"rm *": "deny"}}
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")

	mustWriteFile(t, openCodePermissionSpec, "permissions:\n  deny: [Read, \"Bash(rm:*)\"]\n")
	runSyncOK(t)
	want["read"] = "deny"
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission after a spec change = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")

	mustWriteFile(t, openCodePermissionSpec, "permissions:\n  deny: [Read]\n")
	runSyncOK(t)
	want = map[string]any{"edit": "ask", "read": "deny"}
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission after a rule left = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")

	removeSpecs(t, openCodePermissionSpec)
	runSyncOK(t)
	if got := openCodePermission(t); !reflect.DeepEqual(got, map[string]any{"edit": "ask"}) {
		t.Errorf("permission after the spec left = %#v, want only the hand-written key", got)
	}
	runSyncOK(t, "--check")
}

// A key the user adds after a sync stays through later syncs.
func TestSync_OpenCodePermissionKeepsAKeyAddedAfterSync(t *testing.T) {
	openCodePermissionProject(t, "")
	runSyncOK(t)
	doc := readJSONMap(t, "opencode.json")
	doc["permission"].(map[string]any)["edit"] = "ask"
	writeJSONFile(t, "opencode.json", doc)
	runSyncOK(t)
	runSyncOK(t, "--check")
	removeSpecs(t, openCodePermissionSpec)
	runSyncOK(t)
	if got := openCodePermission(t); !reflect.DeepEqual(got, map[string]any{"edit": "ask"}) {
		t.Errorf("permission = %#v, want only the hand-written key", got)
	}
	runSyncOK(t, "--check")
}

// A bare action sets every permission, so it stays as the catch-all.
func TestSync_OpenCodePermissionKeepsABareAction(t *testing.T) {
	openCodePermissionProject(t, `{"permission": "ask"}`)
	runSyncOK(t)
	want := map[string]any{"*": "ask", "read": "allow", "bash": map[string]any{"rm *": "deny"}}
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission = %#v, want %#v", got, want)
	}
	runSyncOK(t, "--check")
}

// asWholePermissionClaim rewrites the ledger the way a sync that claimed
// the whole permission value left it.
func asWholePermissionClaim(t *testing.T) {
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
	record := state["merged"].(map[string]any)["opencode.json"].(map[string]any)
	var keys []any
	for _, key := range record["keys"].([]any) {
		if p := key.(map[string]any)["path"].([]any); p[0] != "permission" {
			keys = append(keys, key)
		}
	}
	canonical, err := json.Marshal(openCodePermission(t))
	if err != nil {
		t.Fatal(err)
	}
	record["keys"] = append(keys, map[string]any{"path": []any{"permission"}, "sum": adapters.ContentSum(string(canonical))})
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, string(out)+"\n")
}

func openCodePermissionClaims(t *testing.T) [][]string {
	t.Helper()
	var claims [][]string
	for _, key := range readStateFile(".").Merged["opencode.json"].Keys {
		if key.Path[0] == "permission" {
			claims = append(claims, key.Path)
		}
	}
	return claims
}

// A ledger written when sync claimed the whole permission value moves to
// one claim per key, so the spec's keys still leave and the user's stay.
func TestSync_OpenCodePermissionUpgradesAWholeValueClaim(t *testing.T) {
	t.Run("unchanged value", func(t *testing.T) {
		openCodePermissionProject(t, "")
		runSyncOK(t)
		asWholePermissionClaim(t)
		runSyncOK(t)
		if claims := openCodePermissionClaims(t); slices.ContainsFunc(claims, func(p []string) bool { return len(p) == 1 }) || len(claims) != 2 {
			t.Errorf("claims = %v, want one per spec key", claims)
		}
		runSyncOK(t, "--check")
		doc := readJSONMap(t, "opencode.json")
		doc["permission"].(map[string]any)["edit"] = "ask"
		writeJSONFile(t, "opencode.json", doc)
		removeSpecs(t, openCodePermissionSpec)
		runSyncOK(t)
		if got := openCodePermission(t); !reflect.DeepEqual(got, map[string]any{"edit": "ask"}) {
			t.Errorf("permission = %#v, want only the hand-written key", got)
		}
		runSyncOK(t, "--check")
	})
	t.Run("user key added since", func(t *testing.T) {
		openCodePermissionProject(t, "")
		runSyncOK(t)
		asWholePermissionClaim(t)
		doc := readJSONMap(t, "opencode.json")
		doc["permission"].(map[string]any)["edit"] = "ask"
		writeJSONFile(t, "opencode.json", doc)
		mustWriteFile(t, openCodePermissionSpec, "permissions:\n  deny: [Read, \"Bash(rm:*)\"]\n")
		runSyncOK(t)
		want := map[string]any{"edit": "ask", "read": "deny", "bash": map[string]any{"rm *": "deny"}}
		if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
			t.Fatalf("permission = %#v, want %#v", got, want)
		}
		runSyncOK(t, "--check")
		removeSpecs(t, openCodePermissionSpec)
		runSyncOK(t)
		if got := openCodePermission(t); !reflect.DeepEqual(got, map[string]any{"edit": "ask"}) {
			t.Errorf("permission = %#v, want only the hand-written key", got)
		}
		runSyncOK(t, "--check")
	})
}

func openCodeRepoMemoryProject(t *testing.T, handWritten, policy string) string {
	t.Helper()
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	writeFile(t, "opencode.json", handWritten)
	writeFile(t, openCodePermissionSpec, policy)
	return parent
}

func openCodeExternalDirectory(t *testing.T) map[string]any {
	t.Helper()
	got, _ := openCodePermission(t)["external_directory"].(map[string]any)
	return got
}

// A grant a spec wrote leaves with the spec while the memory store is on.
func TestSync_OpenCodePermissionSpecGrantLeavesWithItsSpecInRepoMode(t *testing.T) {
	parent := openCodeRepoMemoryProject(t, "", "permissions:\n  allow: [Read]\nx-opencode:\n  permission:\n    external_directory: allow\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, "opencode.json")) + "/**"
	if got := openCodeExternalDirectory(t); !reflect.DeepEqual(got, map[string]any{"*": "allow", store: "allow"}) {
		t.Fatalf("external_directory = %#v", got)
	}
	removeSpecs(t, openCodePermissionSpec)
	for range 2 {
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
	}
	if got := openCodeExternalDirectory(t); !reflect.DeepEqual(got, map[string]any{store: "allow"}) {
		t.Errorf("external_directory after the spec left = %#v, want only the store", got)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

// The user's external_directory rules stay when specs write other tools
// and the memory store is on.
func TestSync_OpenCodePermissionKeepsUserDirectoryRulesInRepoMode(t *testing.T) {
	parent := openCodeRepoMemoryProject(t, `{"permission": {"edit": "ask", "external_directory": {"/data/**": "deny"}}}`, "permissions:\n  allow: [Read]\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, "opencode.json")) + "/**"
	if got := openCodeExternalDirectory(t); !reflect.DeepEqual(got, map[string]any{"/data/**": "deny", store: "allow"}) {
		t.Fatalf("external_directory = %#v", got)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
	removeSpecs(t, openCodePermissionSpec)
	for range 2 {
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]any{"edit": "ask", "external_directory": map[string]any{"/data/**": "deny", store: "allow"}}
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission after the spec left = %#v, want %#v", got, want)
	}
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	want["external_directory"] = map[string]any{"/data/**": "deny"}
	if got := openCodePermission(t); !reflect.DeepEqual(got, want) {
		t.Errorf("permission after leaving repo mode = %#v, want %#v", got, want)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

// A spec that takes over a tool key the user wrote saves the file first.
func TestSync_OpenCodePermissionSavesAHandKeyASpecReplaces(t *testing.T) {
	openCodePermissionProject(t, `{"permission": {"read": "ask", "edit": "ask"}}`)
	runSyncOK(t)
	if got := readJSONMap(t, "opencode.json")["permission"].(map[string]any)["read"]; got != "allow" {
		t.Fatalf("read = %#v, want the spec's value", got)
	}
	backup := readJSONMap(t, "opencode.json.bak")
	if !reflect.DeepEqual(backup["permission"], map[string]any{"read": "ask", "edit": "ask"}) {
		t.Errorf("opencode.json.bak = %#v, want the hand-written file", backup)
	}
	runSyncOK(t)
	runSyncOK(t, "--check")
}

// A directory rule a spec wrote leaves with the spec while the memory
// store is on, and the store rule stays.
func TestSync_OpenCodePermissionSpecDirectoryRuleLeavesWithItsSpecInRepoMode(t *testing.T) {
	parent := openCodeRepoMemoryProject(t, "", "permissions:\n  allow: [Read]\nx-opencode:\n  permission:\n    external_directory:\n      /tmp/a/**: deny\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, "opencode.json")) + "/**"
	if got := openCodeExternalDirectory(t); !reflect.DeepEqual(got, map[string]any{"/tmp/a/**": "deny", store: "allow"}) {
		t.Fatalf("external_directory = %#v", got)
	}
	removeSpecs(t, openCodePermissionSpec)
	for range 2 {
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
	}
	if got := openCodeExternalDirectory(t); !reflect.DeepEqual(got, map[string]any{store: "allow"}) {
		t.Errorf("external_directory after the spec left = %#v, want only the store", got)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}
