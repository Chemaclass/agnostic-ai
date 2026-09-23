package factory

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

// TestEmit_CapabilityMatrixCoversEveryDeclaredKind enforces the
// invariant that the factory adapter actually emits something for
// every spec kind it declares in caps.Supports, with one deliberate
// exception: KindRule has no per-adapter output at all. Rules reach
// Droid CLI exclusively through the shared AGENTS.md entry-point that
// `sync` writes centrally (see factory.go), a write this per-package
// test never observes because it calls Adapter.Emit directly.
// Declaring KindRule keeps the "unsupported" warning honest (rules do
// reach Droid CLI) without this adapter ever touching a rules file
// itself.
func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	paths := testutil.WalkRel(t, dir)
	type expect struct {
		kind     spec.Kind
		matchers []string
	}
	cases := []expect{
		{spec.KindAgent, []string{".factory/droids/alpha.md", ".factory/droids/beta.md", ".factory/droids/gamma.md"}},
		{spec.KindSkill, []string{".agents/skills/uno/SKILL.md", ".agents/skills/dos/SKILL.md", ".agents/skills/tres/SKILL.md"}},
		{spec.KindMCP, []string{".factory/mcp.json"}},
		{spec.KindHook, []string{".factory/hooks.json"}},
		{spec.KindCommand, []string{".factory/commands/review.md"}},
		{spec.KindSettings, []string{".factory/settings.json"}},
	}
	for _, k := range caps.Supports {
		if k == spec.KindRule {
			continue // delivered by sync's shared entry-point, not this adapter
		}
		found := false
		for _, c := range cases {
			if c.kind != k {
				continue
			}
			for _, m := range c.matchers {
				if pathSetContains(paths, m) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("declared kind %q in caps.Supports has no observable output (paths: %v)", k, paths)
		}
	}
}

// TestEmit_NoCapabilityWarningsForKitSinkBundle asserts that emitting
// every declared kind does not buffer any "unsupported" warning.
func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings for a kit-sink bundle, got %d", got)
	}
}

// TestEmit_UnsupportedKindsWarn asserts ReportUnsupported fires for
// every kind factory does not declare in caps.Supports (Ignore and
// Review; #629 moved Hook, #630 moved Command, and #891 moved Settings
// into the declared set). A future caps.Supports expansion needs to
// delete the matching row here and demonstrate the emit path that backs
// the new claim.
func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)

	entries := []spec.Entry{
		{Kind: spec.KindIgnore, Name: "secrets", Path: "ignore/secrets.md", Body: "*.env"},
		{Kind: spec.KindReview, Name: "api", Path: "reviews/api.md", Body: "review guidance"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 2 {
		t.Errorf("expected 2 capability warnings (ignore, review), got %d", got)
	}
}

// docs.factory.ai/enterprise/hierarchical-settings-and-org-control:
// "Settings are authored in `.factory/` folders, using the same schema
// at every level", levels table row "**Project** |
// `<git-root>/.factory/`". A portable `model` must reach that file, and
// must not take the rest of it with it: only `model` is ever set (#891).
func TestEmit_SettingsWritesProjectTierModelAndPreservesSiblings(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".factory", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"disabledSkills":["legacy"],"telemetry":false}`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"model": "claude-sonnet-4-5"}},
		{Kind: spec.KindSettings, Name: "project", Meta: map[string]any{"model": "gpt-5-codex"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if got["model"] != "gpt-5-codex" {
		t.Errorf("model = %#v, want the last settings spec to win", got["model"])
	}
	if got["telemetry"] != false {
		t.Errorf("foreign key dropped: %#v", got)
	}
	if skills, _ := got["disabledSkills"].([]any); len(skills) != 1 {
		t.Errorf("disabledSkills dropped: %#v", got["disabledSkills"])
	}
}

// The file path honors outputs.factory.conf-file, so the key the
// target page documents actually moves the write.
func TestEmit_SettingsHonorsConfFileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"factory": {ConfFile: "custom/droid.json"},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"model": "gpt-5-codex"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom", "droid.json")); err != nil {
		t.Errorf("conf-file override not honored: %v", err)
	}
}

func pathSetContains(paths []string, needle string) bool {
	needle = filepath.ToSlash(needle)
	for _, p := range paths {
		if strings.Contains(p, needle) {
			return true
		}
	}
	return false
}

// An `x-factory` key on a settings spec reaches `.factory/settings.json`
// untouched, and leaves the keys this adapter manages alone. Factory's
// `sandbox` block is the reason the hatch exists: a real project-tier
// key ("sandbox object Built-in sandboxing for command execution and
// file access", docs.factory.ai/enterprise/hierarchical-settings-and-org-control)
// that maps onto no portable field (#949).
func TestEmit_SettingsCustomTargetKeysReachTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
			"model": "gpt-5-codex",
			"x-factory": map[string]any{
				"sandbox": map[string]any{
					"enabled":    true,
					"mode":       "per-command",
					"filesystem": map[string]any{"denyWrite": []any{"/etc"}},
				},
			},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	sandbox, ok := got["sandbox"].(map[string]any)
	if !ok {
		t.Fatalf("x-factory.sandbox never reached the file: %#v", got)
	}
	if sandbox["enabled"] != true || sandbox["mode"] != "per-command" {
		t.Errorf("sandbox block altered: %#v", sandbox)
	}
	if got["model"] != "gpt-5-codex" {
		t.Errorf("model = %#v, want the managed key untouched", got["model"])
	}
	if _, hasX := got["x-factory"]; hasX {
		t.Errorf("the x-factory wrapper must not be written: %#v", got)
	}
}

// A Factory-only pattern under `x-factory.commandBlocklist` joins the
// translated ones instead of erasing them. The blocklist is the tier
// with no approval path, the one that "can never run ... even under
// full autonomy, auto-run, or --skip-permissions-unsafe", so a
// portable deny dropping out of it is the worst silent edit this
// adapter can make (#966).
func TestEmit_SettingsCustomCommandListJoinsTheTranslatedOne(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{
			"permissions": map[string]any{"deny": []any{"Bash(rm:*)", "Bash(curl)"}},
			"x-factory":   map[string]any{"commandBlocklist": []any{"author-only"}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	want := []any{"rm *", "curl", "author-only"}
	if !reflect.DeepEqual(got["commandBlocklist"], want) {
		t.Errorf("commandBlocklist = %#v, want %#v", got["commandBlocklist"], want)
	}
}

// The three command lists carry the portable policy, in the vendor's
// own grammar. docs.factory.ai/enterprise/hierarchical-settings-and-org-control
// types each as `string[]` of "Shell command patterns", and both
// spellings appear in vendor examples: bare
// (`"commandAllowlist": ["ls", "pwd", "dir"]`, /droid-cli/settings)
// and prefix-glob (`"commandAllowlist": ["npm *", "pnpm *", "make *"]`,
// /enterprise/llm-safety-and-agent-controls). So `Bash(npm:*)` becomes
// `"npm *"` and `Bash(curl)` becomes `"curl"` (#948).
func TestEmit_SettingsWritesTheThreeCommandLists(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(npm:*)", "Bash(ls)"},
			"ask":   []any{"Bash(sudo:*)"},
			"deny":  []any{"Bash(curl)", "Bash(rm -rf:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	var got map[string]any
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	want := map[string][]string{
		"commandAllowlist": {"npm *", "ls"},
		"commandDenylist":  {"sudo *"},
		"commandBlocklist": {"curl", "rm -rf *"},
	}
	for key, patterns := range want {
		list, _ := got[key].([]any)
		if len(list) != len(patterns) {
			t.Errorf("%s = %#v, want %v", key, got[key], patterns)
			continue
		}
		for i, pattern := range patterns {
			if list[i] != pattern {
				t.Errorf("%s[%d] = %#v, want %q", key, i, list[i], pattern)
			}
		}
	}
}

// The trap this issue exists to avoid: portable `deny` must not land
// in `commandDenylist` on name similarity. Factory's denylist prompts,
// and "A denied command can still be run if you explicitly approve it"
// (/droid-cli/settings), asserted again on
// /enterprise/hierarchical-settings-and-org-control ("always require
// confirmation ... use commandBlocklist for a hard block") and
// /enterprise/llm-safety-and-agent-controls ("A denylisted command can
// still run if the user approves it"). A hard deny is the blocklist,
// which has "no prompt and no way to approve them" (#948).
func TestEmit_SettingsDenyGoesToTheBlocklistNotTheDenylist(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Bash(rm -rf /)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	var got map[string]any
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if _, wrong := got["commandDenylist"]; wrong {
		t.Errorf("a portable deny reached commandDenylist, which prompts and can be approved: %#v", got)
	}
	block, _ := got["commandBlocklist"].([]any)
	if len(block) != 1 || block[0] != "rm -rf /" {
		t.Errorf("commandBlocklist = %#v, want the deny rule", got["commandBlocklist"])
	}
}

// Rules merge across settings specs in source order and emit once.
func TestEmit_SettingsCommandListsMergeAcrossSpecsDeduped(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(go test:*)", "Bash(ls)"},
		}}},
		{Kind: spec.KindSettings, Name: "project", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(ls)", "Bash(make:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	var got map[string]any
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	want := []string{"go test *", "ls", "make *"}
	list, _ := got["commandAllowlist"].([]any)
	if len(list) != len(want) {
		t.Fatalf("commandAllowlist = %#v, want %v", got["commandAllowlist"], want)
	}
	for i, pattern := range want {
		if list[i] != pattern {
			t.Errorf("commandAllowlist[%d] = %#v, want %q", i, list[i], pattern)
		}
	}
}

// The coverage note narrows to the scopes with no Factory spelling.
// These three keys are shell-command patterns, so `Read(src/**)` still
// reaches nothing and still says so (#948).
func TestEmit_SettingsNonBashRulesSurfaceCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Bash(go test:*)", "Read(src/**)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions`", "factory", "shell-command"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatalf("the Bash rule must still emit: %v", err)
	}
	if !strings.Contains(string(data), "go test *") {
		t.Errorf("the Bash rule did not reach the file: %s", data)
	}
}

// A spec whose every rule is out of scope raises the note and writes
// no file, the same as before the three lists existed.
func TestEmit_SettingsAllRulesOutOfScopeWritesNoFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"deny": []any{"Read(src/**)", "mcp__github__create_issue"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("a spec with no translatable rule must write no file, got err=%v", err)
	}
}

func TestEmit_SettingsEffortWritesReasoningEffort(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"effort": "max"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".factory", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["reasoningEffort"] != "max" {
		t.Errorf("reasoningEffort = %#v, want max", got["reasoningEffort"])
	}
}
