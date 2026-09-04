package openhands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A rule carrying the cross-tool `globs` field (the Cursor spelling)
// becomes a path-triggered rule (docs.openhands.dev/overview/skills/path):
// a `paths:` frontmatter list, folder form, in the same shared
// `.agents/skills/` tree skills already use.
func TestEmit_Rule_WithGlobsWritesPathTriggeredSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "api-validation", Meta: map[string]any{"globs": "src/api/**/*.ts"}, Body: "Validate all request inputs with zod."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/api-validation/SKILL.md"))
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{"paths:", "src/api/**/*.ts", "Validate all request inputs with zod."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The vendor's own `paths` spelling (matching Claude Code's) wins
// outright over `globs` when both are declared.
func TestEmit_Rule_WithPathsFieldWinsOverGlobs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Meta: map[string]any{
			"globs": "ignored/**",
			"paths": []any{"src/auth/**", "**/*.session.ts"},
		}, Body: "Rotate session tokens."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/auth/SKILL.md"))
	for _, want := range []string{"src/auth/**", "**/*.session.ts"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ignored/**") {
		t.Errorf("globs should not appear once paths wins:\n%s", got)
	}
}

// A rule carrying a source-layout scope, but no explicit globs/paths,
// still becomes a path-triggered rule scoped to `<scope>/**`, the same
// fallback order Copilot's own applyTo derivation uses.
func TestEmit_Rule_WithSourceLayoutScopeWritesPathTriggeredSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "scoped body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/auth/SKILL.md"))
	for _, want := range []string{"backend/api/**", "scoped body"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// An always-on rule (no globs, paths, or scope) reaches OpenHands only
// through the shared AGENTS.md entry-point; this adapter writes
// nothing for it directly.
func TestEmit_Rule_AlwaysOnWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/r1/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write a skill folder for an always-on rule, err=%v", err)
	}
}

// An explicit `alwaysApply: true` wins over a `globs` value, the same
// override every other adapter that reads both fields honors.
func TestEmit_Rule_AlwaysApplyTrueOverridesGlobs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Meta: map[string]any{"globs": "src/**", "alwaysApply": true}, Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/r1/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write a skill folder when alwaysApply overrides globs, err=%v", err)
	}
}

// A path-triggered rule shares outputs.openhands.skills-dir with
// regular skills: the issue's Fix note says no separate
// outputs.openhands.rules-dir workaround is needed.
func TestEmit_Rule_PathTriggeredHonorsSkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"openhands": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Meta: map[string]any{"globs": "src/**"}, Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/r1/SKILL.md")); err != nil {
		t.Errorf("expected override dir to hold the path-triggered rule file: %v", err)
	}
}
