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
