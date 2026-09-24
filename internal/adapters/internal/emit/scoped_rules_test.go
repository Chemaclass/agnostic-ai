package emit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPrepareScopedRules_PreservesConditionsWithoutMutatingSource(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	meta := map[string]any{"scope": "services/payments", "globs": "**/*", "alwaysApply": true, "x-continue": map[string]any{"globs": "services/payments/**/*.go"}}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "money", Meta: meta}})
	cfg := &config.Config{OnUnsupported: "error"}
	prepared, files, err := PrepareScopedRules(b, cfg, "continue")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || len(prepared.Rules) != 1 {
		t.Fatalf("wrong projection: %+v, %+v", prepared, files)
	}
	r := prepared.Rules[0]
	if r.Meta["globs"] != "services/payments/**/*.go" {
		t.Errorf("lost narrow filter: %+v", r.Meta)
	}
	if _, ok := r.Meta["alwaysApply"]; ok {
		t.Fatal("Continue condition must omit alwaysApply")
	}
	if meta["globs"] != "**/*" || meta["alwaysApply"] != true {
		t.Fatal("source mutated")
	}
	if _, _, err := PrepareScopedRules(b, cfg, "codex"); err != nil {
		t.Fatalf("Codex should resolve its own catch-all: %v", err)
	}
}

func TestPrepareScopedRules_RejectsUnrepresentableConditions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, tc := range []struct {
		target string
		meta   map[string]any
	}{
		{"codex", map[string]any{"globs": "payments/**/*.go"}},
		{"claude", map[string]any{"globs": "**/*.go"}},
		{"cursor", map[string]any{"regex": ".*"}},
		{"claude", map[string]any{"globs": "payments/**/*.go", "paths": []string{"payments/**/*.ts"}}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			tc.meta["scope"] = "payments"
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "money", Path: "rules/money.md", Meta: tc.meta}})
			if _, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, tc.target); err == nil || !strings.Contains(err.Error(), "rules/money.md") {
				t.Fatalf("expected actionable error, got %v", err)
			}
		})
	}
}

// TestPrepareScopedRules_AntigravityUsesGlobTrigger pins #1114:
// Antigravity has its own rules directory discovery (`hasScopeFilters`),
// so a scoped rule projects into a glob condition instead of reporting
// "no verified native directory or file-scoped instructions".
func TestPrepareScopedRules_AntigravityUsesGlobTrigger(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "auth", Path: "rules/backend/auth.md", Meta: map[string]any{"scope": "backend"}}})
	prepared, files, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "antigravity")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || len(prepared.Rules) != 1 {
		t.Fatalf("wrong projection: %+v, %+v", prepared, files)
	}
	r := prepared.Rules[0]
	if r.Scope != "backend" {
		t.Errorf("expected scope backend, got %q", r.Scope)
	}
	if r.Meta["alwaysApply"] != false {
		t.Errorf("expected alwaysApply: false, got %+v", r.Meta["alwaysApply"])
	}
	if r.Meta["globs"] != "backend/**" {
		t.Errorf("expected globs: backend/**, got %+v", r.Meta["globs"])
	}
}

// TestPrepareScopedRules_PreservesAntigravityTriggerOverride is the
// unit-level pin for the second review of #1118: a scoped rule's
// `alwaysApply`/`globs` are always force-set from the scope pattern, so
// an `x-antigravity.trigger` override (from `import`'s NativeKeys
// round-trip, or hand-authored directly) must survive alongside them
// instead of being treated as a competing file-matching selector and
// stripped like `applyTo`/`fileMatchPattern`/`glob`/`regex` are.
func TestPrepareScopedRules_PreservesAntigravityTriggerOverride(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	meta := map[string]any{
		"scope":       "backend",
		"alwaysApply": false,
		"x-antigravity": map[string]any{
			"trigger": "manual",
		},
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "manual-only", Meta: meta}})
	prepared, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "antigravity")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Rules) != 1 {
		t.Fatalf("wrong projection: %+v", prepared.Rules)
	}
	r := prepared.Rules[0]
	x, ok := r.Meta["x-antigravity"].(map[string]any)
	if !ok {
		t.Fatalf("expected x-antigravity to survive scoping, got %+v", r.Meta)
	}
	if x["trigger"] != "manual" {
		t.Errorf("expected the trigger override to survive scoping, got %+v", x)
	}
	// The scope's own forced globs still land: the override wins at
	// emit time (ruleTrigger checks it first), so both can coexist here.
	if r.Meta["globs"] != "backend/**" {
		t.Errorf("expected the scope's own forced globs, got %+v", r.Meta["globs"])
	}
}

func TestPrepareScopedRules_LayoutPrecedesMetadata(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "money", Scope: "payments", Meta: map[string]any{"scope": "catalog", "x-codex": map[string]any{"scope": "other"}}, Body: "money convention"}})
	_, files, err := PrepareScopedRules(b, &config.Config{}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.ToSlash(files[0].Path) != "payments/AGENTS.md" {
		t.Fatalf("layout scope lost: %+v", files)
	}
}
