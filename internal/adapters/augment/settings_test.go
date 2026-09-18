package augment

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

func settingsDoc(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, defaultSettingsFile))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return got
}

// Every rule is an object whose `permission` is an object with a
// `type` field. The vendor is explicit that the bare-string form is
// malformed and dropped, so a regression here silently discards the
// whole policy (docs.augmentcode.com/cli/permissions).
func TestEmit_SettingsWritesObjectPermissions(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read", "Bash(npm test:*)"},
			"deny":  []any{"Bash(rm -rf)", "Write"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	want := []any{
		map[string]any{"toolName": "terminal", "shellInputRegex": `^rm -rf$`, "permission": map[string]any{"type": "deny"}},
		map[string]any{"toolName": "write", "permission": map[string]any{"type": "deny"}},
		map[string]any{"toolName": "read", "permission": map[string]any{"type": "allow"}},
		map[string]any{"toolName": "terminal", "shellInputRegex": `^npm test`, "permission": map[string]any{"type": "allow"}},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("toolPermissions =\n%#v\nwant\n%#v", rules, want)
	}
}

// Augment evaluates top-down, first match wins, so deny rules have to
// precede allow rules or a broad allow shadows a narrow deny.
func TestEmit_SettingsOrdersDenyBeforeAllow(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash"},
			"deny":  []any{"Bash(sudo:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	first, _ := rules[0].(map[string]any)
	permission, _ := first["permission"].(map[string]any)
	if permission["type"] != "deny" || first["shellInputRegex"] != `^sudo` {
		t.Errorf("deny rule must come first, got: %#v", rules)
	}
}

// A path-scoped rule has no Augment form: `read`, `edit`, and `write`
// take no path matcher, so flattening one onto the bare tool would
// broaden it into every file. It drops with a note instead. So does
// `ask`, which has no Augment permission type at all.
func TestEmit_SettingsUnmappableRulesSurfaceCoverageNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(src/**)", "Read"},
			"ask":   []any{"Bash(git push:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions`", "`permissions.ask`", "augment", "x-augment"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	if len(rules) != 1 {
		t.Fatalf("only the bare Read rule should survive, got: %#v", rules)
	}
}

// MCP rules translate onto the documented `{tool-name}_{server-name}`
// name. A wildcard has no such name and drops.
func TestEmit_SettingsTranslatesMCPRuleNames(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"mcp__github__delete_repo", "mcp__github__*"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	want := []any{
		map[string]any{"toolName": "delete_repo_github", "permission": map[string]any{"type": "deny"}},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("toolPermissions = %#v", rules)
	}
}

// x-augment.toolPermissions passes through verbatim, ahead of the
// translated rules so an explicit native rule is never shadowed.
func TestEmit_SettingsPassesThroughXAugmentRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
			"permissions": map[string]any{"allow": []any{"Read"}},
			"x-augment": map[string]any{"toolPermissions": []any{
				map[string]any{"toolName": "terminal", "permission": map[string]any{
					"type": "script-policy", "script": "/opt/validate.sh",
				}},
			}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	first, _ := rules[0].(map[string]any)
	permission, _ := first["permission"].(map[string]any)
	if permission["type"] != "script-policy" {
		t.Errorf("x-augment rule must come first, got: %#v", rules)
	}
	if len(rules) != 2 {
		t.Errorf("translated rules should still follow, got: %#v", rules)
	}
}

// `mcpServers`, `hooks`, and `toolPermissions` all merge in one write,
// and every other key in the file survives untouched.
func TestEmit_SettingsPreservesForeignKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, defaultSettingsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark","startupScript":"./boot.sh"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash"},
		}}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := settingsDoc(t, dir)
	if got["theme"] != "dark" || got["startupScript"] != "./boot.sh" {
		t.Errorf("foreign keys were not preserved: %#v", got)
	}
	if _, ok := got["mcpServers"]; !ok {
		t.Errorf("mcpServers should merge in the same write: %#v", got)
	}
	if _, ok := got["toolPermissions"]; !ok {
		t.Errorf("toolPermissions missing: %#v", got)
	}
}

// Multiple settings specs layer in source order and a repeated rule
// emits once, so a base and a project spec do not stack duplicates.
func TestEmit_SettingsLayersInSourceOrder(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read"},
		}}},
		{Kind: spec.KindSettings, Name: "project", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read", "Edit"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules, _ := settingsDoc(t, dir)["toolPermissions"].([]any)
	want := []any{
		map[string]any{"toolName": "read", "permission": map[string]any{"type": "allow"}},
		map[string]any{"toolName": "edit", "permission": map[string]any{"type": "allow"}},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("toolPermissions = %#v", rules)
	}
}

// Augment documents no model key in this file, so a portable model
// must not be invented, and must not vanish silently either.
func TestEmit_SettingsModelSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"model": "sonnet"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); !strings.Contains(note, "`model`") {
		t.Errorf("expected a model coverage note, got: %s", note)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultSettingsFile)); !os.IsNotExist(err) {
		t.Errorf("a model-only settings spec must write no settings file, got err=%v", err)
	}
}
