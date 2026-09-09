package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScopedRules_ReachNativeContext(t *testing.T) {
	cases := []struct{ target, path, selector string }{
		{"claude", ".claude/rules/services/payments/payments.md", "services/payments/**"},
		{"cursor", ".cursor/rules/services/payments/payments.mdc", "alwaysApply: false"},
		{"copilot", ".github/instructions/payments.instructions.md", "services/payments/**"},
		{"cline", ".cline/rules/services/payments/payments.md", "services/payments/**"},
		{"continue", ".continue/rules/services/payments/payments.md", "services/payments/**"},
		{"windsurf", "services/payments/.devin/rules/payments.md", "services/payments/**"},
		{"kiro", ".kiro/steering/payments.md", "services/payments/**"},
		{"trae", "services/payments/.trae/rules/payments.md", "services/payments/**"},
		{"qoder", ".qoder/rules/services/payments/payments.md", "paths:"},
		{"openhands", ".agents/skills/payments/SKILL.md", "services/payments/**"},
		{"codex", "services/payments/AGENTS.md", "payment convention"},
		{"gemini", "services/payments/GEMINI.md", "payment convention"},
		{"amp", "services/payments/AGENTS.md", "payment convention"},
		{"warp", "services/payments/AGENTS.md", "payment convention"},
		{"opencode", "services/payments/AGENTS.md", "payment convention"},
		{"goose", "services/payments/AGENTS.md", "payment convention"},
		{"augment", "services/payments/AGENTS.md", "payment convention"},
		{"factory", "services/payments/AGENTS.md", "payment convention"},
		{"kilo", "services/payments/AGENTS.md", "payment convention"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Path: ".agnostic-ai/rules/payments.md", Body: "payment convention", Meta: map[string]any{"scope": "services/payments", "globs": "**/*", "alwaysApply": true}}})
			a, _ := Get(tc.target)
			cfg := &config.Config{Targets: []string{tc.target}}
			if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
				t.Fatal(err)
			}
			if err := EmitWithProvenance(NewSession(), a, b, cfg, false); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.FromSlash(tc.path))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), tc.selector) || !strings.Contains(string(content), "payment convention") {
				t.Errorf("missing scoped content: %s", content)
			}
		})
	}
}

func TestScopedRules_RejectIncompatibleReaders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		targets []string
		meta    map[string]any
	}{
		{"global AGENTS reader", []string{"codex", "kiro"}, nil},
		{"raw Cursor reader", []string{"cursor", "crush"}, nil},
		{"excluded shared reader", []string{"codex", "amp"}, map[string]any{"target": "codex"}},
		{"different shared scope", []string{"codex", "amp"}, map[string]any{"x-amp": map[string]any{"scope": "catalog"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			meta := tc.meta
			if meta == nil {
				meta = map[string]any{}
			}
			meta["scope"] = "payments"
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Path: "rules/payments.md", Body: "payment convention", Meta: meta}})
			cfg := &config.Config{Targets: tc.targets}
			cfg.Sync.CollisionPolicy = "prefer-spec"
			if err := ValidateScopedRules(cfg, b, tc.targets[:1]); err == nil {
				t.Fatal("expected reader conflict even during partial sync")
			}
			entries, err := os.ReadDir(".")
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("preflight wrote files: %v", entries)
			}
		})
	}
}

func TestScopedRules_UnsupportedNeverEmitsGlobalCopy(t *testing.T) {
	for _, target := range []string{"aider", "zed", "junie", "crush", "jules", "antigravity"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Scope: "payments", Body: "private subtree convention"}})
			a, _ := Get(target)
			cfg := &config.Config{Targets: []string{target}, OnUnsupported: "silent"}
			sess := NewSession()
			sess.StartCapture()
			if err := EmitWithProvenance(sess, a, b, cfg, false); err != nil {
				t.Fatal(err)
			}
			for _, f := range sess.StopCapture() {
				if strings.Contains(f.Content, "private subtree convention") {
					t.Errorf("leaked to %s", f.Path)
				}
			}
			cfg.OnUnsupported = "error"
			if err := ValidateScopedRules(cfg, b, cfg.Targets); err == nil {
				t.Fatal("strict mode must reject unsupported scope")
			}
		})
	}
}

func TestScopedRules_ProtectOwnedPaths(t *testing.T) {
	for _, tc := range []struct {
		name, scope, destination string
		symlink                  bool
	}{
		{"absolute", "/", "", false},
		{"traversal", "services/../payments", "", false},
		{"handwritten", "payments", "payments/AGENTS.md", false},
		{"override", "payments", "payments/AGENTS.override.md", false},
		{"symlink", "payments", "payments", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outside := t.TempDir()
			testutil.Chdir(t, t.TempDir())
			if tc.symlink {
				if err := os.Symlink(outside, tc.destination); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else if tc.destination != "" {
				if err := os.MkdirAll(filepath.Dir(tc.destination), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tc.destination, []byte("keep my instructions"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Meta: map[string]any{"scope": tc.scope}, Body: "new"}})
			cfg := &config.Config{Targets: []string{"codex"}}
			if err := ValidateScopedRules(cfg, b, cfg.Targets); err == nil {
				t.Fatal("expected safe preflight failure")
			}
			if tc.destination != "" && !tc.symlink {
				data, err := os.ReadFile(tc.destination)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "keep my instructions" {
					t.Fatal("overwrote instructions")
				}
			}
		})
	}
}
