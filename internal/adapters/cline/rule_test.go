package cline

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

// Cline's conditional rules use a `paths` array of glob patterns, and
// "Rules without frontmatter are always active"
// (docs.cline.bot/customization/cline-rules). Emitting no frontmatter at
// all promoted every scoped rule to always-on (#639).
func TestEmit_RulePathsFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindRule, Name: "components", Body: "component body",
			Meta: map[string]any{"globs": "src/components/**", "alwaysApply": false},
		},
		{
			Kind: spec.KindRule, Name: "scoped", Path: "rules/backend/scoped.md",
			Scope: "backend", Body: "scoped body",
			Meta: map[string]any{"alwaysApply": false},
		},
		{Kind: spec.KindRule, Name: "always", Body: "always body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	globbed := readRuleFile(t, filepath.Join(dir, ".cline/rules/components.md"))
	if !strings.Contains(globbed, "paths:\n  - src/components/**\n") {
		t.Errorf("expected a paths array from globs:\n%s", globbed)
	}
	scoped := readRuleFile(t, filepath.Join(dir, ".cline/rules/backend/scoped.md"))
	if !strings.Contains(scoped, "paths:\n  - backend/**\n") {
		t.Errorf("expected a paths array from the source-layout scope:\n%s", scoped)
	}
	// An always-on rule stays bare: Cline reads a file with no
	// frontmatter as always active, so writing an empty block would
	// churn every existing file for no behavior change.
	always := readRuleFile(t, filepath.Join(dir, ".cline/rules/always.md"))
	if strings.Contains(always, "paths:") {
		t.Errorf("expected no frontmatter on an always-on rule:\n%s", always)
	}
}

// A rule that declares `alwaysApply: false` with nothing to scope to has
// no Cline conditional. `paths: []` would mean "never activates", which
// is not what the author asked for, so the rule stays always-active and
// the gap surfaces as a coverage note instead.
func TestEmit_RuleWithNoScopeStaysBare(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindRule, Name: "api", Body: "api body",
		Meta: map[string]any{"description": "Applies to API design", "alwaysApply": false},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readRuleFile(t, filepath.Join(dir, ".cline/rules/api.md"))
	if strings.Contains(got, "paths:") {
		t.Errorf("expected no paths key with nothing to scope to:\n%s", got)
	}
}

func readRuleFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
