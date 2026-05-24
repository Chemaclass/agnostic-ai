package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ExecPolicies_WritesSkylarkDSL(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {ExecPolicies: []config.CodexExecPolicy{
			{
				Pattern:       []string{"composer", "test"},
				Decision:      "allow",
				Justification: "Composer scripts are project entrypoints.",
				Match:         []string{"composer test", "composer test -- --filter Foo"},
			},
			{
				Pattern:       []string{"rm", "-rf", "/"},
				Decision:      "forbidden",
				Justification: "Never remove the filesystem root.",
			},
		}},
	}}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/rules/default.rules"))
	if err != nil {
		t.Fatalf("expected .codex/rules/default.rules: %v", err)
	}
	out := string(got)
	for _, want := range []string{
		"# Composer scripts are project entrypoints.",
		`pattern = ["composer", "test"]`,
		`decision = "allow"`,
		"# match: composer test",
		"# Never remove the filesystem root.",
		`pattern = ["rm", "-rf", "/"]`,
		`decision = "forbidden"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("default.rules missing %q in:\n%s", want, out)
		}
	}
}

// Empty / unset exec-policies must not create the file.
func TestEmit_ExecPolicies_NoFileWhenUnset(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex/rules/default.rules")); !os.IsNotExist(err) {
		t.Errorf("expected no default.rules when exec-policies unset, got: %v", err)
	}
}

// Empty pattern is a hard error so a typo does not silently emit a
// no-op rule that would block real commands.
func TestEmit_ExecPolicies_EmptyPatternErrors(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {ExecPolicies: []config.CodexExecPolicy{
			{Pattern: nil, Decision: "allow"},
		}},
	}}
	err := New().Emit(spec.NewBundle(nil), cfg, false)
	if err == nil || !strings.Contains(err.Error(), "pattern must not be empty") {
		t.Errorf("expected pattern-empty error, got: %v", err)
	}
}

func TestEmit_ExecPolicies_BadDecisionErrors(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {ExecPolicies: []config.CodexExecPolicy{
			{Pattern: []string{"x"}, Decision: "maybe"},
		}},
	}}
	err := New().Emit(spec.NewBundle(nil), cfg, false)
	if err == nil || !strings.Contains(err.Error(), "decision must be allow|forbidden|ask") {
		t.Errorf("expected bad-decision error, got: %v", err)
	}
}

// `exec-policies-file` lets a project keep many entries in a separate YAML
// file instead of inlining them in agnostic-ai.yaml. Inline entries (when
// present) come first; file entries append.
func TestEmit_ExecPolicies_LoadsFromFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	policiesFile := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(policiesFile, []byte(`- pattern: ["composer", "fix"]
  decision: allow
  justification: Run formatters.
- pattern: ["sudo"]
  decision: forbidden
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {
			ExecPolicies: []config.CodexExecPolicy{
				{Pattern: []string{"composer", "test"}, Decision: "allow"},
			},
			ExecPoliciesFile: policiesFile,
		},
	}}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".codex/rules/default.rules"))
	out := string(got)
	for _, want := range []string{
		`pattern = ["composer", "test"]`,
		`pattern = ["composer", "fix"]`,
		`pattern = ["sudo"]`,
		`decision = "forbidden"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Inline entry must appear before file entries.
	inlinePos := strings.Index(out, `pattern = ["composer", "test"]`)
	filePos := strings.Index(out, `pattern = ["composer", "fix"]`)
	if inlinePos == -1 || filePos == -1 || inlinePos > filePos {
		t.Errorf("inline entries should render before file entries; inline=%d file=%d", inlinePos, filePos)
	}
}
