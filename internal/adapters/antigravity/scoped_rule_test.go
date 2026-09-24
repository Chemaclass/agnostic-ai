package antigravity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_ScopedRulesLandInASubdirectoryRulesDir pins the routing
// fixed in #1114. Antigravity discovers `.agents/rules` directories
// anywhere in the workspace tree, never a subdirectory inside one:
// "You can place ... a `.agents/rules/` directory ... in any
// subdirectory of your project" (antigravity.google/docs/rules), so a
// scoped rule belongs at `<scope>/.agents/rules/<name>.md`.
func TestEmit_ScopedRulesLandInASubdirectoryRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md", Scope: "backend", Body: "auth body"},
		{Kind: spec.KindRule, Name: "limits", Path: "rules/backend/api/limits.md", Scope: "backend/api", Body: "limits body"},
		{Kind: spec.KindAgent, Name: "delta", Path: "agents/backend/delta.md", Scope: "backend", Body: "delta body"},
		{Kind: spec.KindRule, Name: "root", Path: "rules/root.md", Body: "root body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		"backend/.agents/rules/auth.md",
		"backend/api/.agents/rules/limits.md",
		// A scoped agent stays flat: Antigravity's subdirectory
		// discovery is documented for rules only, and RulesDirectory
		// skips agents here (they emit through emitAgents instead).
		".agents/agents/delta/agent.md",
		".agents/rules/root.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	for _, p := range []string{
		".agents/rules/backend/auth.md",
		".agents/rules/backend/api/limits.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("unreachable nested path still written: %s (err=%v)", p, err)
		}
	}
}

// TestEmit_ScopedRulesFollowTheRulesDirOverride keeps the scope prefix
// attached to whatever rules dir the user configured.
func TestEmit_ScopedRulesFollowTheRulesDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"antigravity": {RulesDir: ".agent/rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md", Scope: "backend", Body: "auth body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/.agent/rules/auth.md")); err != nil {
		t.Errorf("missing backend/.agent/rules/auth.md: %v", err)
	}
}

// TestEmit_ScopeEscapingRootFallsBackToTheRulesDir covers a frontmatter
// `scope: ../x`. Prefixing it to the rules dir would write outside the
// project, so the rule lands unscoped instead of writing nowhere.
func TestEmit_ScopeEscapingRootFallsBackToTheRulesDir(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "escape", Path: "rules/escape.md", Scope: "../outside", Body: "escape body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/rules/escape.md")); err != nil {
		t.Errorf("missing .agents/rules/escape.md: %v", err)
	}
}
