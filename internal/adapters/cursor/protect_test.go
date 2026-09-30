package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func settingsSpec(meta map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: "protected", Path: "settings/protected.yaml", Meta: meta}
}

func protectSpec(decision string, paths ...any) spec.Entry {
	return settingsSpec(map[string]any{"protected": map[string]any{"paths": paths, "decision": decision}})
}

func emitSettings(t *testing.T, entries ...spec.Entry) {
	t.Helper()
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
}

func readCLIConfig(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".cursor", "cli.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func cliRules(t *testing.T, dir, list string) []string {
	t.Helper()
	perms, _ := readCLIConfig(t, dir)["permissions"].(map[string]any)
	return emit.StringSlice(perms[list])
}

func writeCLIConfig(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, ".cursor", "cli.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmit_DenyProtectedPathsBecomeCLIWriteDenyRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	emitSettings(t, protectSpec("deny", "composer.lock", ".github/**", "docs/*.md"))

	want := []string{
		"Write(composer.lock)", "Write(composer.lock/**)",
		"Write(.github/**)",
		"Write(docs/*.md)", "Write(docs/*.md/**)",
	}
	if got := cliRules(t, dir, "deny"); !slices.Equal(got, want) {
		t.Errorf("deny = %v, want %v", got, want)
	}
}

func TestEmit_ProtectedPathsKeepHandWrittenCLIConfig(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeCLIConfig(t, dir, `{"version": 1, "permissions": {"allow": ["Shell(ls)"], "deny": ["Shell(rm)"]}}`)

	emitSettings(t, protectSpec("deny", "composer.lock"))

	doc := readCLIConfig(t, dir)
	if doc["version"] != float64(1) {
		t.Errorf("version = %v, want the hand-written key kept", doc["version"])
	}
	if got := cliRules(t, dir, "allow"); !slices.Equal(got, []string{"Shell(ls)"}) {
		t.Errorf("allow = %v, want the hand-written rule kept", got)
	}
	want := []string{"Shell(rm)", "Write(composer.lock)", "Write(composer.lock/**)"}
	if got := cliRules(t, dir, "deny"); !slices.Equal(got, want) {
		t.Errorf("deny = %v, want %v", got, want)
	}
}

func TestEmit_RemovingAProtectedPathRemovesItsCLIRulesAndKeepsHandWrittenOnes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emitSettings(t, protectSpec("deny", "composer.lock", ".github/**"))
	perms, _ := readCLIConfig(t, dir)["permissions"].(map[string]any)
	perms["deny"] = append(perms["deny"].([]any), "Shell(rm)")
	raw, err := json.Marshal(map[string]any{"permissions": perms})
	if err != nil {
		t.Fatal(err)
	}
	writeCLIConfig(t, dir, string(raw))

	emitSettings(t, protectSpec("deny", ".github/**"))
	if got, want := cliRules(t, dir, "deny"), []string{"Shell(rm)", "Write(.github/**)"}; !slices.Equal(got, want) {
		t.Errorf("after narrowing, deny = %v, want %v", got, want)
	}

	emitSettings(t)
	if got, want := cliRules(t, dir, "deny"), []string{"Shell(rm)"}; !slices.Equal(got, want) {
		t.Errorf("after removal, deny = %v, want %v", got, want)
	}
}

// A rule the user wrote before sync ever protected the path stays theirs.
func TestEmit_AHandWrittenCLIRuleThatMatchesAProtectedPathSurvivesRemoval(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeCLIConfig(t, dir, `{"permissions": {"deny": ["Write(composer.lock)"]}}`)
	emitSettings(t, protectSpec("deny", "composer.lock"))

	emitSettings(t)

	if got := cliRules(t, dir, "deny"); !slices.Equal(got, []string{"Write(composer.lock)"}) {
		t.Errorf("deny = %v, want the hand-written rule kept once", got)
	}
}

func TestEmit_AskProtectedPathsWriteNoCLIConfigAndRaiseANote(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	emitSettings(t, protectSpec("ask", "composer.lock"))
	emit.FlushCoverageNotes()

	if _, err := os.Stat(filepath.Join(dir, ".cursor", "cli.json")); !os.IsNotExist(err) {
		t.Errorf(".cursor/cli.json exists (err=%v), want none for an ask block", err)
	}
	if out := buf.String(); !strings.Contains(out, "protected") || !strings.Contains(out, "no ask list") {
		t.Errorf("want an ask coverage note, got:\n%s", out)
	}
}

func TestEmit_SettingsFieldsCursorDoesNotWriteRaiseNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	emitSettings(t, settingsSpec(map[string]any{
		"model":       "claude-opus-5",
		"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		"x-cursor":    map[string]any{"editor": map[string]any{"vimMode": true}},
	}))
	emit.FlushCoverageNotes()

	if _, err := os.Stat(filepath.Join(dir, ".cursor", "cli.json")); !os.IsNotExist(err) {
		t.Errorf(".cursor/cli.json exists (err=%v), want none without protected paths", err)
	}
	for _, field := range []string{"model", "permissions", "x-cursor"} {
		if !strings.Contains(buf.String(), field) {
			t.Errorf("want a coverage note for %s, got:\n%s", field, buf.String())
		}
	}
}
