package emit

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPrepareScopedRules_PreservesConditionsWithoutMutatingSource(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	meta := map[string]any{"scope": "services/payments", "globs": "services/payments/**", "alwaysApply": true, "x-continue": map[string]any{"globs": "services/payments/**/*.go"}}
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
	if r.Meta["globs"] != "services/payments/**" {
		t.Errorf("scope must include the whole directory: %+v", r.Meta)
	}
	if _, ok := r.Meta["alwaysApply"]; ok {
		t.Fatal("Continue condition must omit alwaysApply")
	}
	if meta["globs"] != "services/payments/**" || meta["alwaysApply"] != true {
		t.Fatal("source mutated")
	}
	if _, _, err := PrepareScopedRules(b, cfg, "codex"); err != nil {
		t.Fatalf("Codex should resolve its own selector: %v", err)
	}
}

func TestPrepareScopedRules_RejectsUnrepresentableConditions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, tc := range []struct {
		target string
		meta   map[string]any
	}{
		{"codex", map[string]any{"globs": "tests/payments/**/*.go"}},
		{"claude", map[string]any{"globs": "/outside/**"}},
		{"cursor", map[string]any{"regex": ".*"}},
		{"claude", map[string]any{"globs": []any{1}}},
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

// A frontmatter scope, resolved for the target, wins over the folder
// (#1430).
func TestPrepareScopedRules_TargetScopePrecedesLayout(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "money", Scope: "payments", Meta: map[string]any{"x-codex": map[string]any{"scope": "other"}}, Body: "money convention"}})
	_, files, err := PrepareScopedRules(b, &config.Config{}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.ToSlash(files[0].Path) != "other/AGENTS.md" {
		t.Fatalf("x-codex scope lost: %+v", files)
	}
}

func TestPrepareScopedRules_UnionRejectsNarrowExternalFiltersWithoutWidening(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Path: "rules/module-a.md", Body: "Module convention.", Meta: map[string]any{"scope": "src/a", "globs": "tests/a/**/*.go"}}})
	for _, policy := range []string{"warn", "silent", "error"} {
		t.Run(policy, func(t *testing.T) {
			ResetCoverageNotes()
			t.Cleanup(ResetCoverageNotes)
			prepared, files, err := PrepareScopedRules(b, &config.Config{OnUnsupported: policy}, "codex")
			if policy == "error" {
				if err == nil || !strings.Contains(err.Error(), "tests/a/**/*.go") {
					t.Errorf("expected the unrepresentable selector in the error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 || len(prepared.Rules) != 0 {
				t.Errorf("union must not be partially emitted or widened: %+v, %+v", prepared, files)
			}
			notes := DrainNotes()
			if policy == "warn" && (len(notes) != 1 || !strings.Contains(notes[0].Reason, "tests/a/**/*.go")) {
				t.Errorf("expected a precise selector warning, got %+v", notes)
			}
			if policy != "warn" && len(notes) != 0 {
				t.Errorf("unexpected notes: %+v", notes)
			}
		})
	}
}

func TestPrepareScopedRules_UnionPreservesNativePatterns(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, tc := range []struct {
		name string
		meta map[string]any
		want []string
	}{
		{"inside scope", map[string]any{"globs": "src/a/**/*.go"}, []string{"src/a/**"}},
		{"several directories", map[string]any{"globs": "tests/a/**,tests/b/**"}, []string{"src/a/**", "tests/a/**", "tests/b/**"}},
		{"both selectors", map[string]any{"globs": "tests/a/**", "paths": []string{"docs/a/**"}}, []string{"docs/a/**", "src/a/**", "tests/a/**"}},
		{"project wide", map[string]any{"globs": "**/*"}, []string{"**"}},
		{"ancestor directory", map[string]any{"globs": "src/**"}, []string{"src/**"}},
		{"root file", map[string]any{"globs": "CHANGELOG.md"}, []string{"CHANGELOG.md", "src/a/**"}},
		{"normalized directory", map[string]any{"globs": "./tests/a/**/*"}, []string{"src/a/**", "tests/a/**"}},
		{"native override", map[string]any{"globs": "ignored/**", "x-claude": map[string]any{"globs": "tests/native/*.go"}}, []string{"src/a/**", "tests/native/*.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.meta["scope"] = "src/a"
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Meta: tc.meta}})
			prepared, files, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "claude")
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 || len(prepared.Rules) != 1 {
				t.Fatalf("wrong union projection: %+v, %+v", prepared, files)
			}
			if got := prepared.Rules[0].Meta["paths"]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("union paths = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestPrepareScopedRules_ScalarSelectorsRejectLiteralCommas(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, target := range []string{"cursor", "copilot", "windsurf", "trae", "antigravity"} {
		t.Run(target, func(t *testing.T) {
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Meta: map[string]any{"scope": "src/a", "paths": []string{"tests/a,**"}}}})
			if _, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, target); err == nil || !strings.Contains(err.Error(), "tests/a,**") {
				t.Errorf("expected unsupported literal comma selector, got %v", err)
			}
			b.Rules[0].Meta["paths"] = []string{"tests/{a,b}/**"}
			if _, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, target); err != nil {
				t.Errorf("brace commas must remain supported: %v", err)
			}
		})
	}
}

func TestPrepareScopedRules_NativeOnlySelectorsDoNotActivateOtherTargets(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "frontend", Scope: "backend", Path: "rules/backend/frontend.md", Meta: map[string]any{"x-continue": map[string]any{"globs": []string{"src/**"}}}}})
	prepared, files, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "continue")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Rules) != 1 || prepared.Rules[0].Scope != "backend" || len(files) != 0 {
		t.Errorf("native selector must retain placement: %+v, %+v", prepared, files)
	}
	if _, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "codex"); err == nil || !strings.Contains(err.Error(), "target-native selectors for continue") {
		t.Errorf("native-only filter must not become a Codex directory rule: %v", err)
	}
	if got := EntryPointRules(b, "codex"); len(got.Rules) != 0 {
		t.Error("native-only rule leaked to Codex root context")
	}
	b.Rules[0].Meta["globs"] = "tests/**"
	if _, files, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "codex"); err != nil || len(files) != 2 {
		t.Errorf("portable fallback must remain available on Codex: %+v, %v", files, err)
	}
}

func TestPrepareScopedRules_UnverifiedNativeSelectorsDoNotBypassDirectoryLimits(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "frontend", Scope: "backend", Meta: map[string]any{"x-codex": map[string]any{"globs": "src/**/*.go"}}}})
	if _, _, err := PrepareScopedRules(b, &config.Config{OnUnsupported: "error"}, "codex"); err == nil || !strings.Contains(err.Error(), "src/**/*.go") {
		t.Errorf("unsupported native field bypassed directory validation: %v", err)
	}
	b.Rules[0].Scope = ""
	if len(b.Rules[0].NativeRuleTargets()) != 0 {
		t.Error("Codex must not claim native glob activation at the root")
	}
}
