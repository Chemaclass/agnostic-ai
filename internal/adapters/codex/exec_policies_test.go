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
		`pattern = ["composer", "test"]`,
		`decision = "allow"`,
		`justification = "Composer scripts are project entrypoints."`,
		`match = ["composer test", "composer test -- --filter Foo"]`,
		`pattern = ["rm", "-rf", "/"]`,
		`decision = "forbidden"`,
		`justification = "Never remove the filesystem root."`,
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

// With no inline policies and no explicit exec-policies-file, the
// emitter falls back to the import-captured overlay so a round-trip
// from a pre-existing .codex/rules/default.rules survives a re-sync
// without forcing the user to add anything to agnostic-ai.yaml.
func TestEmit_ExecPolicies_AutoLoadsOverlay(t *testing.T) {
	dir := testutil.TempCwd(t)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, []byte(`- pattern: ["composer", "test"]
  decision: allow
  justification: Captured from prior .codex/rules/default.rules
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// No outputs.codex.exec-policies, no exec-policies-file. Adapter
	// should still emit because the overlay is present.
	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/rules/default.rules"))
	if err != nil {
		t.Fatalf("expected emit to pick up overlay: %v", err)
	}
	if !strings.Contains(string(got), `pattern = ["composer", "test"]`) {
		t.Errorf("rendered policies missing overlay entry:\n%s", got)
	}
}

// Regression for #283: the captured `codex.exec-policies-header.txt`
// sidecar renders as a `# ...` block above the first rule, separated
// by a blank line so a re-import detects it as the file-level header
// (not as the first rule's justification).
func TestEmit_ExecPolicies_RendersCapturedFileHeader(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai/overlays"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml"), []byte(`- pattern: ["composer", "test"]
  decision: allow
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies-header.txt"), []byte("Codex exec-policy rules for Phel.\nKeep the list short.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Emit(spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".codex/rules/default.rules"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "# Codex exec-policy rules for Phel.\n# Keep the list short.\n\nprefix_rule(") {
		t.Errorf("captured header missing or not blank-line separated from first rule:\n%s", out)
	}
}

// When the user has inline entries, the overlay is ignored — the user
// has opted in to a declarative source of truth.
func TestEmit_ExecPolicies_InlineSuppressesOverlay(t *testing.T) {
	dir := testutil.TempCwd(t)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, []byte(`- pattern: ["from-overlay"]
  decision: allow
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Outputs: map[string]config.Output{
		"codex": {ExecPolicies: []config.CodexExecPolicy{
			{Pattern: []string{"from-inline"}, Decision: "allow"},
		}},
	}}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".codex/rules/default.rules"))
	out := string(got)
	if !strings.Contains(out, `pattern = ["from-inline"]`) {
		t.Errorf("inline missing:\n%s", out)
	}
	if strings.Contains(out, `pattern = ["from-overlay"]`) {
		t.Errorf("overlay should be suppressed when inline entries set:\n%s", out)
	}
}
