package continueai

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Continue documents `name`, `description`, `globs`, `regex`, and
// `alwaysApply` on a rule file (continuedev/continue,
// docs/customize/deep-dives/rules.mdx). Emitting no frontmatter at all
// left every scoped rule always-on (#639).
func TestEmit_RuleActivationFrontmatter(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindRule, Name: "typescript", Body: "typescript body",
		Meta: map[string]any{
			"description": "TypeScript conventions",
			"globs":       "**/*.ts",
			"alwaysApply": false,
			"x-continue":  map[string]any{"regex": "^import .* from '.*';$"},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".continue/rules/typescript.md"))
	for _, want := range []string{
		"name: typescript\n",
		"description: TypeScript conventions\n",
		"globs: \"**/*.ts\"\n",
		"alwaysApply: false\n",
		"regex: ^import .* from '.*';$\n",
		"typescript body",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in .continue/rules/typescript.md:\n%s", want, got)
		}
	}
}

// A rule with a source-layout scope and no explicit globs still gets a
// glob, so it stops loading on every request.
func TestEmit_ScopedRuleGetsGlobs(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md",
		Scope: "backend", Body: "auth body",
		Meta: map[string]any{"alwaysApply": false},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".continue/rules/backend/auth.md"))
	if !strings.Contains(got, "globs: backend/**\n") {
		t.Errorf("expected globs derived from the scope:\n%s", got)
	}
}
