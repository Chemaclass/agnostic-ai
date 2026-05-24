package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestImportFromCodex_ExecPolicies_CapturedToOverlay(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex/rules/default.rules"), `# Always allow build/test commands.
prefix_rule(
    pattern = ["composer", "test"],
    decision = "allow",
)
# match: composer test
# match: composer test -- --filter Foo

prefix_rule(
    pattern = ["rm", "-rf", "/"],
    decision = "forbidden",
)
`)
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml")
	data, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatalf("expected overlay written: %v", err)
	}
	var policies []config.CodexExecPolicy
	if err := yaml.Unmarshal(data, &policies); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, data)
	}
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d:\n%s", len(policies), data)
	}

	// First policy: composer test, allow, justification + matches.
	p := policies[0]
	if !equalStrings(p.Pattern, []string{"composer", "test"}) {
		t.Errorf("policy[0] pattern = %v, want [composer test]", p.Pattern)
	}
	if p.Decision != "allow" {
		t.Errorf("policy[0] decision = %q, want allow", p.Decision)
	}
	if !strings.Contains(p.Justification, "Always allow build/test commands.") {
		t.Errorf("policy[0] justification missing comment: %q", p.Justification)
	}
	if !equalStrings(p.Match, []string{"composer test", "composer test -- --filter Foo"}) {
		t.Errorf("policy[0] match = %v", p.Match)
	}

	// Second policy: rm -rf /, forbidden.
	p = policies[1]
	if !equalStrings(p.Pattern, []string{"rm", "-rf", "/"}) {
		t.Errorf("policy[1] pattern = %v", p.Pattern)
	}
	if p.Decision != "forbidden" {
		t.Errorf("policy[1] decision = %q", p.Decision)
	}
}

// Missing default.rules is silent — most projects do not have one.
func TestImportFromCodex_ExecPolicies_NoFile_NoOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml")
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Errorf("expected no overlay when default.rules absent, got: %v", err)
	}
}

// Empty default.rules captures nothing (no surprise overlay).
func TestImportFromCodex_ExecPolicies_EmptyFile_NoOverlay(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".codex/rules/default.rules"), "# only comments\n")
	if err := importFromCodex(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, ".agnostic-ai/overlays/codex.exec-policies.yaml")
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Errorf("expected no overlay when default.rules has no rules, got: %v", err)
	}
}

// Regression for #277: when justification + match are expressed as
// kwargs inside the prefix_rule(...) call (the form codex CLI itself
// emits when authors hand-write the file), they must round-trip into
// the captured overlay alongside pattern + decision.
func TestParseCodexExecPolicies_InlineKwargs(t *testing.T) {
	body := `prefix_rule(
    pattern = ["rm", "-rf", "/"],
    decision = "forbidden",
    justification = "Never remove the filesystem root.",
    match = ["rm -rf /"],
)
`
	got, err := parseCodexExecPolicies(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	p := got[0]
	if !equalStrings(p.Pattern, []string{"rm", "-rf", "/"}) {
		t.Errorf("pattern = %v", p.Pattern)
	}
	if p.Decision != "forbidden" {
		t.Errorf("decision = %q", p.Decision)
	}
	if p.Justification != "Never remove the filesystem root." {
		t.Errorf("justification = %q", p.Justification)
	}
	if !equalStrings(p.Match, []string{"rm -rf /"}) {
		t.Errorf("match = %v", p.Match)
	}
}

// parseCodexExecPolicies handles single-line prefix_rule too.
func TestParseCodexExecPolicies_SingleLineCall(t *testing.T) {
	body := `prefix_rule(pattern = ["sudo"], decision = "ask")` + "\n"
	got, err := parseCodexExecPolicies(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if !equalStrings(got[0].Pattern, []string{"sudo"}) {
		t.Errorf("pattern = %v", got[0].Pattern)
	}
	if got[0].Decision != "ask" {
		t.Errorf("decision = %q", got[0].Decision)
	}
}
