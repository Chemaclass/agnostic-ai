package adapters

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScopedRules_ReachNativeContext(t *testing.T) {
	cases := []struct{ target, path, selector string }{
		{"claude", ".claude/rules/services/payments/payments.md", "services/payments/**"},
		{"cursor", ".cursor/rules/services/payments/payments.mdc", "alwaysApply: false"},
		{"copilot", ".github/instructions/payments.instructions.md", "services/payments/**"},
		{"cline", ".clinerules/services/payments/payments.md", "services/payments/**"},
		{"continue", ".continue/rules/services/payments/payments.md", "services/payments/**"},
		{"windsurf", "services/payments/.devin/rules/payments.md", "services/payments/**"},
		{"antigravity", "services/payments/.agents/rules/payments.md", "services/payments/**"},
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
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "payments", Path: ".agnostic-ai/rules/payments.md", Body: "payment convention", Meta: map[string]any{"scope": "services/payments", "globs": "services/payments/**/*.go", "alwaysApply": true}}})
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

func TestScopedRules_UnionReachesSourceAndTests(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex", "gemini", "amp", "warp", "opencode", "goose", "augment", "factory", "kilo", "cursor", "copilot", "cline", "continue", "windsurf", "trae", "antigravity", "kiro", "qoder", "openhands"} {
		t.Run(target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Path: ".agnostic-ai/rules/module-a.md", Body: "Module and test convention.", Meta: map[string]any{"scope": "src/a", "globs": []string{"tests/a/**"}}}})
			cfg := &config.Config{Targets: []string{target}, OnUnsupported: "error"}
			if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
				t.Fatal(err)
			}
			a, _ := Get(target)
			if err := EmitWithProvenance(NewSession(), a, b, cfg, false); err != nil {
				t.Fatal(err)
			}
			testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "testdata", "scope-union", target))
		})
	}
}

func TestScopedRules_UnionReadersCheckEveryDestination(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Body: "Module convention.", Meta: map[string]any{"scope": "src/a", "globs": "tests/a/**"}}})
	cfg := &config.Config{Targets: []string{"codex", "cursor", "amp"}, OnUnsupported: "error"}
	if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
		t.Fatal(err)
	}
	b.Rules[0].Meta["x-amp"] = map[string]any{"globs": "tests/b/**"}
	if err := ValidateScopedRules(cfg, b, cfg.Targets[:1]); err == nil || !strings.Contains(err.Error(), "tests/") {
		t.Errorf("expected conflict at the union's extra directory, got %v", err)
	}
}

func TestScopedRules_UnionReaderIgnoresUnrelatedUnsupportedSelectors(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "module-a", Body: "Module convention.", Meta: map[string]any{"scope": "src/a", "globs": "tests/a/**"}},
		{Kind: spec.KindRule, Name: "other", Body: "Other convention.", Meta: map[string]any{"scope": "other", "target": "cursor", "x-cursor": map[string]any{"regex": ".*"}}},
	})
	cfg := &config.Config{Targets: []string{"codex", "cursor"}, OnUnsupported: "warn"}
	defer ResetCoverageNotes()
	if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
		t.Errorf("unrelated selector must follow warn policy, got %v", err)
	}
}

func TestScopedRules_ListSelectorsPreserveLiteralCommas(t *testing.T) {
	for _, tc := range []struct{ target, key string }{{"kiro", "fileMatchPattern"}, {"continue", "globs"}, {"claude", "paths"}, {"cline", "paths"}, {"qoder", "paths"}, {"openhands", "paths"}} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Body: "Guide.", Meta: map[string]any{"scope": "src/a", "paths": []string{"tests/a,**"}}}})
			cfg := &config.Config{Targets: []string{tc.target}, OnUnsupported: "error"}
			a, _ := Get(tc.target)
			sess := NewSession()
			sess.StartCapture()
			if err := EmitWithProvenance(sess, a, b, cfg, false); err != nil {
				t.Fatal(err)
			}
			files := sess.StopCapture()
			if len(files) != 1 {
				t.Fatalf("expected one native rule, got %+v", files)
			}
			var meta map[string]any
			front := strings.Split(files[0].Content, "---")
			if len(front) < 3 {
				t.Fatal("native rule has no frontmatter")
			}
			if err := yaml.Unmarshal([]byte(front[1]), &meta); err != nil {
				t.Fatal(err)
			}
			want := []any{"src/a/**", "tests/a,**"}
			if !reflect.DeepEqual(meta[tc.key], want) {
				t.Errorf("%s = %#v, want %#v", tc.key, meta[tc.key], want)
			}
		})
	}
}

func TestScopedRules_UnionReaderSkipsUnsupportedRuleAtAddedDestination(t *testing.T) {
	for _, policy := range []string{"warn", "silent"} {
		t.Run(policy, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{
				{Kind: spec.KindRule, Name: "module-a", Body: "Module convention.", Meta: map[string]any{"scope": "src/a", "globs": "tests/a/**"}},
				{Kind: spec.KindRule, Name: "other", Body: "Other convention.", Meta: map[string]any{"scope": "tests/a", "target": "cursor", "x-cursor": map[string]any{"regex": ".*"}}},
			})
			cfg := &config.Config{Targets: []string{"codex", "cursor"}, OnUnsupported: policy}
			defer ResetCoverageNotes()
			if err := ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
				t.Errorf("skipped rule must follow %s policy, got %v", policy, err)
			}
		})
	}
}

func TestScopedRules_UnionCatchAllKeepsProjectWideNativeSelector(t *testing.T) {
	for _, target := range []string{"claude", "cursor", "copilot", "cline", "continue", "windsurf", "trae", "antigravity", "kiro", "qoder", "openhands"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "module-a", Body: "Union convention.", Meta: map[string]any{"scope": "src/a", "globs": "**/*"}}})
			cfg := &config.Config{Targets: []string{target}, OnUnsupported: "error"}
			a, _ := Get(target)
			sess := NewSession()
			sess.StartCapture()
			if err := EmitWithProvenance(sess, a, b, cfg, false); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range sess.StopCapture() {
				if strings.Contains(file.Content, "Union convention.") {
					found = true
					if strings.Contains(file.Content, "src/a/**") || !strings.Contains(file.Content, "**") {
						t.Errorf("catch-all union narrowed at %s: %s", file.Path, file.Content)
					}
				}
			}
			if !found {
				t.Fatal("no native rule carries the catch-all union")
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
	for _, target := range []string{"aider", "zed", "junie", "crush", "jules"} {
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
		{"glob delimiter", "payments,catalog", "", false},
		{"glob negation", "!payments", "", false},
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

// The companion exemption holds only while the Claude adapter replaces
// the companion; one it keeps would load the scoped text a second time.
func TestValidateScopedRules_CompanionOnlyWhenClaudeReplacesIt(t *testing.T) {
	const conflict = "alternate instructions conflict"
	cases := []struct {
		name    string
		targets []string
		meta    map[string]any
		cfg     func(*config.Config)
		wantErr string
	}{
		{name: "claude writes the scope", targets: []string{"claude", "codex"}},
		{name: "claude not a target", targets: []string{"codex"}, wantErr: conflict},
		{name: "no claude rule in the scope", targets: []string{"claude", "codex"}, meta: map[string]any{"target": "codex"}, wantErr: conflict},
		{name: "rules file layout", targets: []string{"claude", "codex"}, wantErr: "rules-file", cfg: func(c *config.Config) {
			c.Outputs = map[string]config.Output{"claude": {RulesFile: "CLAUDE.md"}}
		}},
		{name: "unmanaged companion", targets: []string{"claude", "codex"}, wantErr: conflict, cfg: func(c *config.Config) {
			c.Sync.Unmanaged = []string{"services/api/CLAUDE.md"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			if err := os.MkdirAll("services/api", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("services/api/CLAUDE.md", []byte("@AGENTS.md\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			meta := map[string]any{"scope": "services/api"}
			for k, v := range tc.meta {
				meta[k] = v
			}
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "api", Body: "Use integer minor units.", Meta: meta}})
			cfg := &config.Config{Targets: tc.targets}
			if tc.cfg != nil {
				tc.cfg(cfg)
			}

			err := ValidateScopedRules(cfg, b, cfg.Targets)
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("ValidateScopedRules() = %v, want an error containing %q", err, tc.wantErr)
			}
			if tc.wantErr == "" && err != nil {
				t.Errorf("ValidateScopedRules() = %v, want the companion allowed", err)
			}
		})
	}
}
