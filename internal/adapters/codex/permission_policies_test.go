package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func permissionPolicyConfig(t *testing.T, body string) *config.Config {
	t.Helper()
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

func TestEmit_PermissionPoliciesTranslateConfigAndPortableLists(t *testing.T) {
	testutil.TempCwd(t)
	cfg := permissionPolicyConfig(t, `outputs:
  codex:
    exec-policies-from-permissions: true
  claude:
    settings:
      permissions:
        allow: ["Bash(npm run check)", "Bash(npx vitest run:*)", "Bash(git diff:*)"]
        deny: ["Bash(rm -rf:*)"]
        ask: ["Bash(git push:*)"]
`)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "portable", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Bash(go test:*)", "Bash(npm run check)"}}}}})
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(defaultExecPoliciesFile)
	if err != nil {
		t.Fatalf("translated policy missing: %v", err)
	}
	for _, want := range []string{`pattern = ["npm", "run", "check"]`, `pattern = ["npx", "vitest", "run"]`, `pattern = ["git", "diff"]`, `pattern = ["go", "test"]`, `decision = "forbidden"`, `decision = "prompt"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("policy lacks %s:\n%s", want, data)
		}
	}
	if got := strings.Count(string(data), `pattern = ["npm", "run", "check"]`); got != 1 {
		t.Errorf("duplicate config/spec policy: %d", got)
	}
}

func TestBashPermissionPrefix_ReadsOneTrailingWildcard(t *testing.T) {
	cases := map[string][]string{
		"Bash(git diff *)":   {"git", "diff"},
		"Bash(git diff:*)":   {"git", "diff"},
		"Bash(git diff)":     {"git", "diff"},
		"Bash(git diff*)":    nil,
		"Bash(git * diff)":   nil,
		"Bash(git diff * *)": nil,
		"Bash(git diff *:*)": nil,
		"Bash(*)":            nil,
		"Bash( *)":           nil,
	}
	for rule, want := range cases {
		got, ok := bashPermissionPrefix(rule)
		if ok != (want != nil) || !slices.Equal(got, want) {
			t.Errorf("bashPermissionPrefix(%q) = %q, %v; want %q", rule, got, ok, want)
		}
	}
}

func TestEmit_PermissionPoliciesRejectUnsupportedRuleWithItsSource(t *testing.T) {
	cases := []string{`Bash(echo "hello world")`, "Bash(git * status)", "Bash(git status && echo ok)", "Bash(git status\necho ok)", "Bash(FOO=bar git status)", "Bash(git\u00a0diff)", "Bash"}
	for _, rule := range cases {
		t.Run(rule, func(t *testing.T) {
			testutil.TempCwd(t)
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			cfg := permissionPolicyConfig(t, "on-unsupported: error\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n")
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "portable", Path: "settings/security.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{rule}}}}})
			err := New().Emit(emit.NewSession(), b, cfg, false)
			if err == nil || !strings.Contains(err.Error(), rule) || !strings.HasPrefix(err.Error(), "settings/security.yaml: ") {
				t.Errorf("unsupported rule should lead with its source and name the exact rule, got %v", err)
			}
		})
	}
}

func TestEmit_PermissionPoliciesNoteOtherToolRulesWithoutFailing(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	var notes strings.Builder
	previous := emit.Warner
	emit.Warner = &notes
	t.Cleanup(func() { emit.Warner = previous })
	cfg := permissionPolicyConfig(t, "on-unsupported: error\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\", \"WebFetch\"]\n")
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "security", Path: "settings/security.yaml", Meta: map[string]any{"permissions": map[string]any{"deny": []any{"Read(.env)", "mcp__github__delete_repo"}}}}})
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatalf("rules for other tools failed the sync: %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "`permissions` on 1 settings has no effect on codex") || strings.Contains(notes.String(), "Read(.env)") {
		t.Errorf("rules for other tools should share one permissions note:\n%s", notes.String())
	}
	data, err := os.ReadFile(defaultExecPoliciesFile)
	if err != nil || !strings.Contains(string(data), `pattern = ["git", "diff"]`) {
		t.Errorf("Bash rule not translated: %v\n%s", err, data)
	}
}

func TestEmit_PermissionPoliciesNativeSourcesWin(t *testing.T) {
	cases := []struct{ name, field, file, body string }{
		{"inline", "exec-policies: [{pattern: [git, diff], decision: prompt}]", "", ""},
		{"file", "exec-policies-file: policies.yaml", "policies.yaml", "- pattern: [git, diff]\n  decision: forbidden\n"},
		{"overlay", "", execPoliciesOverlayPath, "- pattern: [git, diff]\n  decision: forbidden\n"},
		{"empty file", "exec-policies-file: policies.yaml", "policies.yaml", "[]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if tc.file != "" {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, tc.file)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := permissionPolicyConfig(t, "outputs:\n  codex:\n    exec-policies-from-permissions: true\n    "+tc.field+"\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\", \"Bash(npm run check)\"]\n")
			if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(defaultExecPoliciesFile)
			if tc.name == "empty file" && os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `decision = "allow"`) || strings.Contains(string(data), "npm") {
				t.Errorf("automatic policy overrides native intent:\n%s", data)
			}
		})
	}
}

func TestEmit_PermissionPoliciesNoteTheNativeSourceThatWins(t *testing.T) {
	cases := []struct{ field, file, source string }{
		{"exec-policies: []", "", "outputs.codex.exec-policies"},
		{"exec-policies-file: policies.yaml", "policies.yaml", "policies.yaml"},
		{"", execPoliciesOverlayPath, execPoliciesOverlayPath},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			var notes strings.Builder
			previous := emit.Warner
			emit.Warner = &notes
			t.Cleanup(func() { emit.Warner = previous })
			if tc.file != "" {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, tc.file)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.file), []byte("- {pattern: [git], decision: forbidden}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := permissionPolicyConfig(t, "outputs:\n  codex:\n    exec-policies-from-permissions: true\n    "+tc.field+"\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\"]\n")
			if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, true); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			want := "note: codex: exec policies come from " + tc.source + ", so outputs.codex.exec-policies-from-permissions has no effect"
			if !strings.Contains(notes.String(), want) {
				t.Errorf("notes lack %q:\n%s", want, notes.String())
			}
		})
	}
}

func TestEmit_PermissionPoliciesCoverageHonorsWarnSilentAndDryRun(t *testing.T) {
	for _, mode := range []string{"warn", "silent", "error"} {
		t.Run(mode, func(t *testing.T) {
			testutil.TempCwd(t)
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			var notes strings.Builder
			previous := emit.Warner
			emit.Warner = &notes
			t.Cleanup(func() { emit.Warner = previous })
			cfg := permissionPolicyConfig(t, "on-unsupported: "+mode+"\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\", \"Bash(git * log)\"]\n")
			err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, true)
			if (err != nil) != (mode == "error") {
				t.Errorf("mode %s error = %v", mode, err)
			}
			emit.FlushCoverageNotes()
			if mode == "warn" && (!strings.Contains(notes.String(), "Bash(git * log)") || !strings.Contains(notes.String(), config.ConfigFileName)) {
				t.Errorf("missing exact coverage note: %s", notes.String())
			}
			if mode == "silent" && notes.Len() != 0 {
				t.Errorf("silent reports notes: %s", notes.String())
			}
			if strings.Contains(notes.String(), "Bash(git diff:*)") {
				t.Errorf("translated rule reported as dropped: %s", notes.String())
			}
			if _, err := os.Stat(defaultExecPoliciesFile); !os.IsNotExist(err) {
				t.Errorf("dry run created policy: %v", err)
			}
		})
	}
}

func TestEmit_PermissionPoliciesStayOptIn(t *testing.T) {
	testutil.TempCwd(t)
	cfg := permissionPolicyConfig(t, "outputs:\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\"]\n")
	if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(defaultExecPoliciesFile); !os.IsNotExist(err) {
		t.Errorf("permissions grant without opt-in: %v", err)
	}
}

func TestEmit_PermissionPoliciesEmptyInlineListIsAuthoritativeAfterConfigLoad(t *testing.T) {
	for _, source := range []string{"base", "local", "overlay"} {
		t.Run(source, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			body := "targets: [codex]\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n    exec-policies: []\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\"]\n"
			if source == "local" {
				body = strings.Replace(body, "exec-policies: []", "exec-policies: [{pattern: [git], decision: forbidden}]", 1)
			}
			if err := os.WriteFile(filepath.Join(dir, config.ConfigFileName), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if source == "local" {
				if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFileName), []byte("outputs:\n  codex:\n    exec-policies: []\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if source == "overlay" {
				if err := os.MkdirAll(filepath.Dir(execPoliciesOverlayPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(execPoliciesOverlayPath, []byte("- {pattern: [git], decision: forbidden}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			policies := cfg.Outputs["codex"].ExecPolicies
			if policies == nil || len(policies) != 0 {
				t.Fatalf("loaded explicit empty policies = %#v", policies)
			}
			if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(defaultExecPoliciesFile); !os.IsNotExist(err) {
				t.Errorf("explicit empty list generated policies: %v", err)
			}
		})
	}
}

func TestEmit_PermissionPoliciesSkipExactAllowListedBeforeCoveringWildcard(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	var notes strings.Builder
	previous := emit.Warner
	emit.Warner = &notes
	t.Cleanup(func() { emit.Warner = previous })
	cfg := permissionPolicyConfig(t, "outputs:\n  codex:\n    exec-policies-from-permissions: true\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git status)\", \"Bash(git:*)\"]\n")
	if err := New().Emit(emit.NewSession(), spec.Bundle{}, cfg, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(defaultExecPoliciesFile)
	if err != nil || !strings.Contains(string(data), `pattern = ["git", "status"]`) {
		t.Errorf("exact rule not translated: %v\n%s", err, data)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(notes.String(), "Bash(git status)") {
		t.Errorf("a later wildcard already allows extra arguments, but the exact rule was named:\n%s", notes.String())
	}
}

func TestEmit_PermissionPoliciesNameExactAllowRulesCodexWidens(t *testing.T) {
	for _, mode := range []string{"warn", "error", "silent"} {
		t.Run(mode, func(t *testing.T) {
			testutil.TempCwd(t)
			emit.ResetCoverageNotes()
			t.Cleanup(emit.ResetCoverageNotes)
			var notes strings.Builder
			previous := emit.Warner
			emit.Warner = &notes
			t.Cleanup(func() { emit.Warner = previous })
			cfg := permissionPolicyConfig(t, "on-unsupported: "+mode+"\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git push)\", \"Bash(git diff:*)\", \"Bash(go *)\", \"Bash(go test)\", \"Bash(npm run check)\", \"Bash(tar x)\"]\n        deny: [\"Bash(rm -rf)\", \"Bash(git push --force)\"]\n        ask: [\"Bash(npm run:*)\", \"Bash(tar)\"]\n")
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "security", Path: "settings/security.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Bash(make lint)"}}}}})
			err := New().Emit(emit.NewSession(), b, cfg, false)
			if mode == "error" {
				if err == nil || !strings.Contains(err.Error(), "Bash(make lint)") {
					t.Errorf("error = %v, want source widening", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("sync: %v", err)
			}
			data, err := os.ReadFile(defaultExecPoliciesFile)
			if err != nil || !strings.Contains(string(data), `pattern = ["git", "push"]`) {
				t.Errorf("exact rule not translated: %v\n%s", err, data)
			}
			emit.FlushCoverageNotes()
			got := notes.String()
			if mode == "silent" {
				if got != "" {
					t.Errorf("silent reports notes: %s", got)
				}
				return
			}
			for _, want := range []string{
				"note: codex: settings/security.yaml: permissions.allow rule Bash(make lint) becomes a Codex prefix rule",
				"note: codex: " + config.ConfigFileName + ": permissions.allow rule Bash(git push) becomes a Codex prefix rule",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("notes lack %q:\n%s", want, got)
				}
			}
			for _, unwidened := range []string{"Bash(git diff:*)", "Bash(go test)", "Bash(go *)", "Bash(rm -rf)", "Bash(npm run check)", "Bash(tar x)", "Bash(git push --force)"} {
				if strings.Contains(got, unwidened) {
					t.Errorf("%s does not widen in Codex but was named:\n%s", unwidened, got)
				}
			}
		})
	}
}
