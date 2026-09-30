package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func protectSpec(decision string, paths ...any) spec.Entry {
	return settingsEntry(map[string]any{"protected": map[string]any{"paths": paths, "decision": decision}})
}

func emitSettings(t *testing.T, entries ...spec.Entry) {
	t.Helper()
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
}

func readPermissions(t *testing.T, dir string) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions map[string][]string `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Permissions
}

// addHandWrittenRule edits settings.json the way a user would.
func addHandWrittenRule(t *testing.T, dir, list, rule string) {
	t.Helper()
	path := filepath.Join(dir, ".claude", "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	doc := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
	}
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	rules, _ := perms[list].([]any)
	perms[list] = append(rules, rule)
	doc["permissions"] = perms
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmit_RemovingAProtectedBlockRemovesItsRulesAndKeepsHandWrittenOnes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, protectSpec("deny", "composer.lock"))
	addHandWrittenRule(t, dir, "deny", "Edit(/mine.txt)")

	emitSettings(t, settingsEntry(map[string]any{"model": "opus"}))

	deny := readPermissions(t, dir)["deny"]
	if slices.Contains(deny, "Edit(/composer.lock)") {
		t.Errorf("deny = %v, still holds the removed protected rule", deny)
	}
	if !slices.Contains(deny, "Edit(/mine.txt)") {
		t.Errorf("deny = %v, lost the hand-written rule", deny)
	}
}

func TestEmit_RemovingTheOnlySettingsSpecRemovesProtectedRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, protectSpec("ask", "composer.lock"))

	emitSettings(t)

	if ask := readPermissions(t, dir)["ask"]; slices.Contains(ask, "Edit(/composer.lock)") {
		t.Errorf("ask = %v, still holds the removed protected rule", ask)
	}
}

func TestEmit_ChangingDenyToAskMovesTheRule(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, protectSpec("deny", "composer.lock", ".github/**"))

	emitSettings(t, protectSpec("ask", "composer.lock"))

	perms := readPermissions(t, dir)
	if len(perms["deny"]) != 0 {
		t.Errorf("deny = %v, want the protected rules gone", perms["deny"])
	}
	if want := []string{"Edit(/composer.lock)"}; !slices.Equal(perms["ask"], want) {
		t.Errorf("ask = %v, want %v", perms["ask"], want)
	}
}

// A rule the user wrote before sync ever protected the path stays theirs.
func TestEmit_AHandWrittenRuleThatMatchesAProtectedPathSurvivesRemoval(t *testing.T) {
	dir := testutil.TempCwd(t)
	addHandWrittenRule(t, dir, "deny", "Edit(/composer.lock)")
	emitSettings(t, protectSpec("deny", "composer.lock"))

	emitSettings(t)

	if deny := readPermissions(t, dir)["deny"]; !slices.Equal(deny, []string{"Edit(/composer.lock)"}) {
		t.Errorf("deny = %v, want the hand-written rule kept once", deny)
	}
}
