package windsurf

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
	data, err := os.ReadFile(filepath.Join(dir, defaultConfigFile))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return got
}

// Devin's project config takes `permissions` with three lists
// (docs.devin.ai/cli/reference/permissions). Portable rules translate
// onto its own vocabulary: Read and Write pass through, Edit collapses
// onto Write, a Bash prefix rule becomes Exec, WebFetch becomes Fetch,
// and an mcp__ rule passes through untouched.
func TestEmit_SettingsTranslatesPortableRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(src/**)", "Bash(go test:*)", "Edit(src/**)", "mcp__github__list_issues"},
			"deny":  []any{"Bash(rm:*)", "Write(.env*)"},
			"ask":   []any{"WebFetch(domain:example.test)", "Bash"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	want := map[string][]any{
		"allow": {"Read(src/**)", "Exec(go test)", "Write(src/**)", "mcp__github__list_issues"},
		"deny":  {"Exec(rm)", "Write(.env*)"},
		"ask":   {"Fetch(domain:example.test)", "exec"},
	}
	for key, values := range want {
		if !reflect.DeepEqual(permissions[key], values) {
			t.Errorf("%s = %#v, want %#v", key, permissions[key], values)
		}
	}
}

// Devin accepts `web_search` in the three permission lists as of
// v3000.10.21 (2026-09-10): "`web_search` can now be used as a tool
// name in `permissions.deny` / `permissions.ask` / `permissions.allow`;
// previously it was rejected and web searches were always
// auto-approved." A portable `WebSearch` deny reached nothing before
// that mapping existed (#951).
func TestEmit_SettingsTranslatesWebSearch(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"WebSearch"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["deny"], []any{"web_search"}) {
		t.Errorf("deny = %#v, want [web_search]", permissions["deny"])
	}
}

// Devin's `write` tool became a documented permission-rule name in
// CLI v3000.11.1 (September 21, 2026): "Custom subagent profiles and
// skills can grant the `write` tool using `allowed-tools`, and
// permission rules recognize it." A bare `Write` rule now maps onto
// `write`, distinct from `Edit`'s `edit`, instead of both collapsing
// onto the same name (#1022).
func TestEmit_SettingsTranslatesBareWriteAndEditDistinctly(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Write", "Edit"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["allow"], []any{"write", "edit"}) {
		t.Errorf("allow = %#v, want [write edit]", permissions["allow"])
	}
}

// `webfetch` stays out of the permission vocabulary on purpose. It
// appears in the lifecycle-hooks tool table and a user-tier
// `disabled_tools` example, neither of which governs `permissions`.
// `web_search` is the proof: it was a live tool from May 2026 and
// `permissions` still rejected it until September. Only a sentence
// about `permissions` licenses a permissions entry.
func TestEmit_SettingsDropsBareWebFetch(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"WebFetch", "Read"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["deny"], []any{"read"}) {
		t.Errorf("deny = %#v, want only [read]", permissions["deny"])
	}
}

// The subagent `allowed-tools` vocabulary is keyed separately from the
// permission vocabulary. `/cli/subagents` enumerates no tool list at
// all, so nothing there licenses `web_search` either way, and one
// shared map would let a future edit for one surface change the other.
func TestAgent_AllowedToolsExcludesWebSearch(t *testing.T) {
	if _, ok := devinTool["WebSearch"]; ok {
		t.Errorf("devinTool must not carry WebSearch: the subagent docs enumerate no tool vocabulary")
	}
	if _, ok := devinPermissionTool["WebSearch"]; !ok {
		t.Errorf("devinPermissionTool must carry WebSearch")
	}
}

// A rule with no faithful Devin spelling is never guessed at. Devin's
// Exec is a prefix matcher with no exact-command form, so an exact
// Bash rule would widen; it drops and folds into one coverage note.
func TestEmit_SettingsUntranslatableRuleSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)", "Bash(git status)", "Task"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions`", "windsurf", "x-windsurf"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["allow"], []any{"Read(**)"}) {
		t.Errorf("allow = %#v, want only the translatable rule", permissions["allow"])
	}
}

// `agent.model` is user-only: "Only `permissions`, `read_config_from`,
// and `hooks` are available in project configs"
// (docs.devin.ai/cli/reference/configuration/config-file). A portable
// model must not reach this file, and must not vanish silently either.
func TestEmit_SettingsModelStaysOutOfProjectConfig(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"model": "swe-1-6-fast"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); !strings.Contains(note, "`model`") {
		t.Errorf("expected a model coverage note, got: %s", note)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultConfigFile)); !os.IsNotExist(err) {
		t.Errorf("a model-only settings spec must write no project config, got err=%v", err)
	}
}

// The file also holds `read_config_from` and `hooks`, the only other
// keys Devin accepts in a project config, plus whatever a user or a
// newer CLI put there. Only `permissions` is ever set.
func TestEmit_SettingsPreservesForeignKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, defaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"read_config_from":{"claude":false},"hooks":{"PreToolUse":[]},"permissions":{"allow":["Exec(docker)"],"unknown_future_key":true}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash(sudo:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := settingsDoc(t, dir)
	readConfigFrom, _ := got["read_config_from"].(map[string]any)
	if readConfigFrom["claude"] != false {
		t.Errorf("read_config_from was not preserved: %#v", got["read_config_from"])
	}
	if _, ok := got["hooks"]; !ok {
		t.Errorf("hooks was not preserved: %#v", got)
	}
	permissions, _ := got["permissions"].(map[string]any)
	if permissions["unknown_future_key"] != true {
		t.Errorf("native permissions sibling was not preserved: %#v", permissions)
	}
	if !reflect.DeepEqual(permissions["deny"], []any{"Exec(sudo)"}) {
		t.Errorf("deny = %#v", permissions["deny"])
	}
}

// Multiple settings specs layer in source order, deduplicated, the same
// way they already do for every other target with this surface.
func TestEmit_SettingsLayersInSourceOrder(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)"},
		}}},
		{Kind: spec.KindSettings, Name: "project", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)", "Bash(npm run:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["allow"], []any{"Read(**)", "Exec(npm run)"}) {
		t.Errorf("allow = %#v", permissions["allow"])
	}
}

// An exact `Bash(cmd)` has no exact Devin spelling, but the safe
// direction differs by list. Devin's changelog for v3000.10.31 states
// "A command deny such as `Exec(rm)` blocks the command even when a
// broader `ask` or `allow` rule covers the whole tool", so widening a
// deny is strictly more restrictive and must not drop. The same
// widening on `allow` or `ask` approves commands the author never
// listed, so those keep dropping.
func TestEmit_SettingsExactBashDenyWidensInsteadOfDropping(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny":  []any{"Bash(rm -rf /)"},
			"allow": []any{"Bash(git status)"},
			"ask":   []any{"Bash(docker run)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	permissions, _ := settingsDoc(t, dir)["permissions"].(map[string]any)
	if !reflect.DeepEqual(permissions["deny"], []any{"Exec(rm -rf /)"}) {
		t.Errorf("deny = %#v, want the widened Exec rule", permissions["deny"])
	}
	if _, ok := permissions["allow"]; ok {
		t.Errorf("allow = %#v, want an exact Bash rule dropped", permissions["allow"])
	}
	if _, ok := permissions["ask"]; ok {
		t.Errorf("ask = %#v, want an exact Bash rule dropped", permissions["ask"])
	}
}

// A deny rule that widened still reports nothing missing, while an
// allow rule outside the vocabulary still folds into the coverage note.
func TestEmit_SettingsExactBashDenyRaisesNoCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash(rm -rf /)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); strings.Contains(note, "`permissions`") {
		t.Errorf("a widened deny must not report a gap, got: %s", note)
	}
}

// An `x-windsurf` key on a settings spec reaches `.devin/config.json`
// untouched, and leaves the managed `permissions` alone (#949).
func TestEmit_SettingsCustomTargetKeysReachTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"permissions": map[string]any{"allow": []any{"Read(src/**)"}},
		"x-windsurf":  map[string]any{"read_config_from": "main"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := settingsDoc(t, dir)
	if got["read_config_from"] != "main" {
		t.Errorf("x-windsurf.read_config_from never reached the file: %#v", got)
	}
	perms, _ := got["permissions"].(map[string]any)
	if allow, _ := perms["allow"].([]any); len(allow) != 1 {
		t.Errorf("permissions = %#v, want the managed key untouched", got["permissions"])
	}
	if _, hasX := got["x-windsurf"]; hasX {
		t.Errorf("the x-windsurf wrapper must not be written: %#v", got)
	}
}

// A settings spec carrying only an `x-windsurf` key still writes the
// file. Devin takes no portable `model`, so before the hatch such a
// spec reached nothing at all (#949).
func TestEmit_SettingsCustomTargetKeysAloneWriteTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{
		"x-windsurf": map[string]any{"read_config_from": "main"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := settingsDoc(t, dir); got["read_config_from"] != "main" {
		t.Errorf("x-windsurf.read_config_from never reached the file: %#v", got)
	}
}
