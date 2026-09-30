package claude

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitSettingsWith(t *testing.T, cfg *config.Config, entries ...spec.Entry) {
	t.Helper()
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
}

func permissionsSpec(list string, rules ...any) map[string]any {
	return map[string]any{"permissions": map[string]any{list: rules}}
}

func TestEmit_RemovingAPortableRuleRemovesItAndKeepsHandWrittenOnes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, settingsEntry(permissionsSpec("deny", "Bash(rm:*)", "Bash(curl:*)")))
	addHandWrittenRule(t, dir, "deny", "Bash(mine:*)")

	emitSettings(t, settingsEntry(permissionsSpec("deny", "Bash(curl:*)")))

	if deny, want := readPermissions(t, dir)["deny"], []string{"Bash(mine:*)", "Bash(curl:*)"}; !slices.Equal(deny, want) {
		t.Errorf("deny = %v, want %v", deny, want)
	}
}

func TestEmit_MovingAPortableRuleFromAllowToDenyLeavesNoAllow(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, settingsEntry(permissionsSpec("allow", "Bash(git push:*)")))

	emitSettings(t, settingsEntry(permissionsSpec("deny", "Bash(git push:*)")))

	perms := readPermissions(t, dir)
	if len(perms["allow"]) != 0 || !slices.Equal(perms["deny"], []string{"Bash(git push:*)"}) {
		t.Errorf("permissions = %v, want the rule in deny only", perms)
	}
}

func TestEmit_RemovingAnXClaudeRuleRemovesIt(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, settingsEntry(map[string]any{"x-claude": permissionsSpec("deny", "AuthorOnly")}))

	emitSettings(t, settingsEntry(map[string]any{"model": "opus"}))

	if deny := readPermissions(t, dir)["deny"]; len(deny) != 0 {
		t.Errorf("deny = %v, want the x-claude rule gone", deny)
	}
}

func TestEmit_RemovingAConfigRuleRemovesIt(t *testing.T) {
	dir := testutil.TempCwd(t)
	withRule := &config.Config{Outputs: map[string]config.Output{target: {Settings: &config.ClaudeSettings{Permissions: &config.ClaudePermissions{Ask: []string{"Bash(npm publish:*)"}}}}}}
	emitSettingsWith(t, withRule)

	emitSettingsWith(t, &config.Config{}, settingsEntry(map[string]any{"model": "opus"}))

	if ask := readPermissions(t, dir)["ask"]; len(ask) != 0 {
		t.Errorf("ask = %v, want the config rule gone", ask)
	}
}

// Before this record existed, sync wrote rules it could not tell apart
// from the user's. The first sync keeps every rule and adopts the ones
// the specs still produce, so a later removal takes them out.
func TestEmit_FirstSyncWithoutARecordAdoptsOnlyRulesTheSpecsProduce(t *testing.T) {
	dir := testutil.TempCwd(t)
	addHandWrittenRule(t, dir, "deny", "Bash(rm:*)")
	addHandWrittenRule(t, dir, "deny", "Bash(mine:*)")

	emitSettings(t, settingsEntry(permissionsSpec("deny", "Bash(rm:*)")))
	if deny := readPermissions(t, dir)["deny"]; !slices.Equal(deny, []string{"Bash(rm:*)", "Bash(mine:*)"}) {
		t.Fatalf("first sync deny = %v, want both rules kept", deny)
	}

	emitSettings(t, settingsEntry(map[string]any{"model": "opus"}))
	if deny := readPermissions(t, dir)["deny"]; !slices.Equal(deny, []string{"Bash(mine:*)"}) {
		t.Errorf("deny = %v, want only the hand-written rule", deny)
	}
}

// A captured overlay is the user's settings: its rules are never
// recorded as sync's, so removing a spec does not remove them.
func TestEmit_OverlayRulesSurviveRemovingASpecWithTheSameRule(t *testing.T) {
	dir := testutil.TempCwd(t)
	overlay := filepath.Join(dir, ".agnostic-ai", "overlays", "claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlay), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(`{"permissions":{"deny":["Bash(rm:*)"]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emitSettings(t, settingsEntry(permissionsSpec("deny", "Bash(rm:*)", "Bash(curl:*)")))

	emitSettings(t)

	if deny := readPermissions(t, dir)["deny"]; !slices.Equal(deny, []string{"Bash(rm:*)"}) {
		t.Errorf("deny = %v, want the overlay rule only", deny)
	}
}
