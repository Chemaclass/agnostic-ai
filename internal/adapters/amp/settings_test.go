package amp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func ampSettingsDoc(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, defaultMCPFile))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return got
}

func captureNotes(t *testing.T) *strings.Builder {
	t.Helper()
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	return buf
}

// An `x-amp` block on a settings spec carries Amp's own settings keys
// into `.amp/settings.json` verbatim. `amp.tools.disable` is the key
// that earned the hatch: ampcode.com/cli-settings.schema.json declares
// it as an array of strings, "Disable specific tools by name".
func TestEmit_Settings_CustomKeysReachSettingsFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{
			"x-amp": map[string]any{
				"amp.tools.disable": []any{"builtin:oracle", "Bash*"},
				"amp.showCosts":     false,
			},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := ampSettingsDoc(t, dir)
	if !reflect.DeepEqual(got["amp.tools.disable"], []any{"builtin:oracle", "Bash*"}) {
		t.Errorf("amp.tools.disable = %#v", got["amp.tools.disable"])
	}
	if got["amp.showCosts"] != false {
		t.Errorf("amp.showCosts = %#v, want false", got["amp.showCosts"])
	}
}

// Settings and MCP servers share one file, so they must share one
// write. A sync that carries both lands both keys.
func TestEmit_Settings_SharesOneWriteWithMCPServers(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{
			"x-amp": map[string]any{"amp.tools.disable": []any{"builtin:oracle"}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := ampSettingsDoc(t, dir)
	servers, _ := got[ampMCPKey].(map[string]any)
	if _, ok := servers["fs"]; !ok {
		t.Errorf("%s = %#v, want the fs server", ampMCPKey, got[ampMCPKey])
	}
	if !reflect.DeepEqual(got[toolsDisableKey], []any{"builtin:oracle"}) {
		t.Errorf("%s = %#v", toolsDisableKey, got[toolsDisableKey])
	}
}

// `amp.mcpServers` is excluded from the settings hatch. It is built
// from MCP specs, which carry their own per-server `x-amp` block, so a
// blanket set from a settings spec would silently replace every server
// the MCP specs contributed.
func TestEmit_Settings_CustomKeysCannotReplaceMCPServers(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{
			"x-amp": map[string]any{ampMCPKey: map[string]any{"ghost": map[string]any{"command": "no"}}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	servers, _ := ampSettingsDoc(t, dir)[ampMCPKey].(map[string]any)
	if _, ok := servers["ghost"]; ok {
		t.Errorf("a settings spec must not inject an MCP server, got %#v", servers)
	}
	if _, ok := servers["fs"]; !ok {
		t.Errorf("the MCP spec's server must survive, got %#v", servers)
	}
}

// Amp's settings schema has no tool allow-list and no ask tier: "By
// default, Amp does not ask for approval before running tools"
// (ampcode.com/docs/tools). Those two lists must say so rather than
// vanish.
func TestEmit_Settings_AllowAndAskNoteFieldNoOp(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := captureNotes(t)

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)"},
			"ask":   []any{"Write(.env*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions.allow`", "`permissions.ask`", "amp"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, defaultMCPFile)); !os.IsNotExist(err) {
		t.Errorf("a settings spec with nothing to write must write no file, err=%v", err)
	}
}

// `amp.tools.disable` is the deny surface, but the tool vocabulary is
// unpublished: "You can see Amp's builtin tools by running `amp tools
// list` in the CLI" (ampcode.com/docs/tools). A portable deny name is
// not guessed at; the note points at the hatch instead.
func TestEmit_Settings_DenyNotesMissingVocabulary(t *testing.T) {
	buf := captureNotes(t)

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash(rm:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions.deny`", "amp tools list", toolsDisableKey} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
}

// Amp's settings schema declares twenty properties and none of them
// selects a model, so a portable `model` reaches nothing here.
func TestEmit_Settings_ModelNotesFieldNoOp(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := captureNotes(t)

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{"model": "gpt-6-astra"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); !strings.Contains(note, "`model`") {
		t.Errorf("expected a model coverage note, got: %s", note)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultMCPFile)); !os.IsNotExist(err) {
		t.Errorf("a model-only settings spec must write no file, err=%v", err)
	}
}

// Every other key in `.amp/settings.json` survives a settings write,
// the same guarantee the MCP write already gives.
func TestEmit_Settings_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, defaultMCPFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"amp.skills.path":"~/my-skills"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{
			"x-amp": map[string]any{toolsDisableKey: []any{"builtin:oracle"}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := ampSettingsDoc(t, dir)
	if got["amp.skills.path"] != "~/my-skills" {
		t.Errorf("amp.skills.path = %#v, want the pre-existing value", got["amp.skills.path"])
	}
}

// Settings ride `outputs.amp.mcp-file` because they share the file it
// names. One override, one file.
func TestEmit_Settings_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"amp": {MCPFile: ".amp/custom.json"},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "policy", Meta: map[string]any{
			"x-amp": map[string]any{toolsDisableKey: []any{"builtin:oracle"}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".amp/custom.json")); err != nil {
		t.Errorf("expected settings at the override path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultMCPFile)); !os.IsNotExist(err) {
		t.Errorf("expected no file at the default path, err=%v", err)
	}
}
