package kilo

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

func TestEmit_SettingsWritesProjectModelAndPreservesKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, "kilo.jsonc")
	if err := os.WriteFile(path, []byte(`{"provider":{"openai":{}},"shared_agent_board":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "openai/example"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "openai/example" || got["shared_agent_board"] != false || got["provider"] == nil {
		t.Errorf("project settings were not merged: %#v", got)
	}
}

// Kilo Code's CLI reads a per-tool permission map: "Permissions are
// configured under the `permission` key in `kilo.jsonc`", with three
// actions and glob patterns "matched against the tool's arguments"
// (kilo.ai/docs/getting-started/settings/auto-approving-actions). The
// portable lists reached nothing here before #890.
func TestEmit_SettingsWritesPermissionMap(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(npm run:*)", "Read(docs/*)"},
			"deny":  []any{"Bash(rm -rf *)", "Edit(*.env)"},
			"ask":   []any{"WebSearch"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	want := map[string]map[string]any{
		"bash":      {"npm run *": "allow", "rm -rf *": "deny"},
		"read":      {"docs/*": "allow"},
		"edit":      {"*.env": "deny"},
		"websearch": {"*": "ask"},
	}
	for tool, patterns := range want {
		if !reflect.DeepEqual(permission[tool], toAnyMap(patterns)) {
			t.Errorf("permission[%q] = %#v, want %#v", tool, permission[tool], patterns)
		}
	}
}

// "Put broad fallbacks first and exceptions after them", because "the
// last matching rule wins". Both encoders sort map keys, and `*` sorts
// ahead of every tool name and command pattern, so the emitted order
// already satisfies the contract. Hold that, since it is load-bearing.
func TestEmit_SettingsPermissionPutsCatchAllFirst(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(git:*)"},
			"ask":   []any{"Bash"},
			"deny":  []any{"Bash(rm -rf:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	catchAll := strings.Index(raw, `"*": "ask"`)
	git := strings.Index(raw, `"git *": "allow"`)
	rm := strings.Index(raw, `"rm -rf *": "deny"`)
	if catchAll < 0 || git < 0 || rm < 0 {
		t.Fatalf("expected all three bash rules, got:\n%s", raw)
	}
	if catchAll > git || catchAll > rm {
		t.Errorf("the catch-all must sort before every exception, got:\n%s", raw)
	}
}

// A rule with no Kilo key is never guessed at. Claude's
// `WebFetch(domain:...)` shorthand is the interesting case: Kilo
// matches "the tool's arguments", so a literal `domain:` pattern would
// match no URL and the rule would be inert.
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
			"allow": []any{"Read(docs/*)", "WebFetch(domain:example.test)", "NotebookEdit"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions`", "kilo", "x-kilo.permission"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	if len(permission) != 1 || permission["read"] == nil {
		t.Errorf("only the translatable rule may land, got: %#v", permission)
	}
}

// "Each MCP tool's permission key is its namespaced name:
// `{server}_{tool}`", so a portable `mcp__github__list_issues` has an
// exact Kilo spelling and does not drop.
func TestEmit_SettingsPermissionTranslatesMCPRule(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"mcp__github__list_issues"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	if !reflect.DeepEqual(permission["github_list_issues"], map[string]any{"*": "deny"}) {
		t.Errorf("mcp rule = %#v", permission["github_list_issues"])
	}
}

// A rule repeated across two lists resolves to the most restrictive
// action, since only one value can survive in the emitted map.
func TestEmit_SettingsPermissionPrefersTheStricterAction(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(rm:*)"},
			"deny":  []any{"Bash(rm:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	bash, _ := permission["bash"].(map[string]any)
	if bash["rm *"] != "deny" {
		t.Errorf("bash = %#v, want the deny to win", bash)
	}
}

// `x-kilo.permission` on a settings spec replaces the translated map
// outright, the same deal it already gets on an agent.
func TestEmit_SettingsNativePermissionWins(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
			"permissions": map[string]any{"allow": []any{"Read(docs/*)"}},
			"x-kilo": map[string]any{"permission": map[string]any{
				"lsp": map[string]any{"*": "allow"},
			}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "kilo.jsonc"))), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	if permission["read"] != nil {
		t.Errorf("x-kilo.permission must replace the translated map, got: %#v", permission)
	}
	if permission["lsp"] == nil {
		t.Errorf("x-kilo.permission did not reach the file: %#v", permission)
	}
}

// A user's own permission entry for a tool agnostic-ai does not set
// survives the merge, the same way `skills.urls` already does.
func TestEmit_SettingsPermissionPreservesForeignTools(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, "kilo.jsonc")
	if err := os.WriteFile(path, []byte(`{"permission":{"doom_loop":{"*":"ask"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash(rm:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	permission, _ := got["permission"].(map[string]any)
	if permission["doom_loop"] == nil {
		t.Errorf("a foreign permission entry was dropped: %#v", permission)
	}
	if permission["bash"] == nil {
		t.Errorf("the managed entry did not land: %#v", permission)
	}
}

// An agent's portable `tools` allowlist now reaches Kilo Code's own
// access control instead of vanishing: deny everything, then re-allow
// the names that translated. The catch-all must come first, since the
// last matching key wins.
func TestEmit_AgentToolsBecomePermissionFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "reviewer", Body: "body", Meta: map[string]any{
			"description": "reviews code",
			"tools":       []any{"Read", "Grep", "Glob"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".kilo/agents/reviewer.md"))
	if strings.Contains(got, "tools:") {
		t.Errorf("tools has no Kilo Code key and must not be written, got:\n%s", got)
	}
	for _, want := range []string{"permission:", `"*": deny`, "read: allow", "grep: allow", "glob: allow"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, `"*": deny`) > strings.Index(got, "read: allow") {
		t.Errorf("the catch-all must come before every allow, got:\n%s", got)
	}
}

// An agent whose whole tools list is untranslatable keeps its default
// permissions. A bare `{"*": "deny"}` would lock it out of everything,
// which nobody asked for.
func TestEmit_AgentAllToolsUnmappedWritesNoPermission(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "odd", Body: "body", Meta: map[string]any{
			"tools": []any{"NotebookEdit"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, ".kilo/agents/odd.md")); strings.Contains(got, "permission:") {
		t.Errorf("an all-unmapped tools list must write no permission map, got:\n%s", got)
	}
}

// toAnyMap widens a test fixture so reflect.DeepEqual compares it
// against a decoded JSON object.
func toAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
