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

func TestEmit_Rule_WithGlobsWritesFlatPathTriggeredRule(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "api-validation", Meta: map[string]any{"globs": "src/api/**/*.ts"}, Body: "Validate all request inputs with zod."},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/api-validation.md"))
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/api-validation/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("expected the rule to have no skill folder, got %v", err)
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{"name: path-rule/api-validation", "paths:", "src/api/**/*.ts", "Validate all request inputs with zod."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_Rule_AvoidsReservedFlatNames(t *testing.T) {
	dir := testutil.TempCwd(t)
	var entries []spec.Entry
	want := map[string]string{
		"agent": "agent-path-rule", "agents": "agents-path-rule", "claude": "claude-path-rule",
		"gemini": "gemini-path-rule", "README": "README-path-rule-2",
		"skill": "skill-path-rule", "readme-path-rule": "readme-path-rule",
	}
	for name := range want {
		entries = append(entries, spec.Entry{Kind: spec.KindRule, Name: name, Meta: map[string]any{"paths": "docs/**"}, Body: name})
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for name, filename := range want {
		got := readFile(t, filepath.Join(dir, ".agents/skills", filename+".md"))
		if !strings.Contains(got, "name: path-rule/"+name+"\n") || !strings.Contains(got, "docs/**") {
			t.Errorf("rule %s lost name or paths:\n%s", name, got)
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
	got := readFile(t, filepath.Join(dir, ".agents/skills/auth.md"))
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
func TestEmit_Rule_WithSourceLayoutScopeWritesFlatPathTriggeredRule(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Scope: "backend/api", Body: "scoped body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/auth.md"))
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
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/r1.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write a file for an always-on rule, err=%v", err)
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
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/r1.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write a file when alwaysApply overrides globs, err=%v", err)
	}
}

// A glob (or paths list) that resolves to nothing but catch-all
// patterns stays always-on: it scopes to every file, so treating it as
// path-triggered would only relocate an always-on rule out of
// AGENTS.md, never actually scope it. This also covers a real
// round-trip: Claude Code's own rule frontmatter carries only `paths`
// (no `alwaysApply`), so a rule authored with `globs: "**/*"` and
// `alwaysApply: true` loses the explicit always-on marker once it
// re-syncs through Claude and gets re-imported, leaving only a
// catch-all `paths` value behind.
func TestEmit_Rule_CatchAllGlobStaysAlwaysOn(t *testing.T) {
	dir := testutil.TempCwd(t)

	cases := []struct {
		name string
		meta map[string]any
	}{
		{"globs-star-star-slash-star", map[string]any{"globs": "**/*"}},
		{"globs-star-star", map[string]any{"globs": "**"}},
		{"globs-star", map[string]any{"globs": "*"}},
		{"paths-star-star-slash-star", map[string]any{"paths": []any{"**/*"}}},
		{"mixed-catch-all-and-scoped", map[string]any{"paths": []any{"src/**", "**/*"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := []spec.Entry{{Kind: spec.KindRule, Name: c.name, Meta: c.meta, Body: "rule body"}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".agents/skills", c.name+".md")); !os.IsNotExist(err) {
				t.Errorf("adapter should not write a file for a catch-all glob, err=%v", err)
			}
		})
	}
}

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
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/r1.md")); err != nil {
		t.Errorf("expected override dir to hold the path-triggered rule file: %v", err)
	}
}
