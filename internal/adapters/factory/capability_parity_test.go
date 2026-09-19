package factory

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// A settings spec carrying only permission lists writes no file at all,
// and says why. Factory names `commandAllowlist`, `commandDenylist`,
// and `commandBlocklist` without a rule grammar or a stated difference
// between the two deny keys, so the rules are not guessed at (#891).
func TestEmit_SettingsPermissionsSurfaceCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
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
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`permissions`", "factory", "commandAllowlist"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("a permissions-only settings spec must write no file, got err=%v", err)
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
