package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_CodexPoliciesReportMissingAndConflictingPermissions(t *testing.T) {
	dir := budgetProject(t, `targets: [claude, codex]
outputs:
  codex:
    exec-policies:
      - {pattern: [npm, run], decision: allow}
      - {pattern: [git], decision: allow}
      - {pattern: [git, push], decision: forbidden}
      - {pattern: [rm], decision: prompt}
  claude:
    settings:
      permissions:
        allow: ["Bash(npm run check)", "Bash(npx vitest run:*)", "Bash(git:*)"]
        deny: ["Bash(rm -rf:*)"]
`)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/settings/security.yaml"), "name: security\npermissions:\n  deny: [\"Bash(curl:*)\"]\n")
	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("policy drift is advisory: %v\n%s", err, out)
	}
	lines := strings.Join(findingLines(out, "LINT021"), "\n")
	for _, want := range []string{"Bash(npx vitest run:*)", "Bash(git:*)", "Bash(rm -rf:*)", "Bash(curl:*)", "agnostic-ai.yaml", "security.yaml"} {
		if !strings.Contains(lines, want) {
			t.Errorf("drift findings lack %s:\n%s", want, out)
		}
	}
	if strings.Contains(lines, "Bash(npm run check)") {
		t.Errorf("matching broader prefix incorrectly flagged:\n%s", out)
	}
}

func TestLint_CodexPoliciesReadFileAndHonorStrictMode(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\noutputs:\n  codex:\n    exec-policies-file: policies.yaml\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(npm run check)\"]\n        deny: [\"Bash(rm -rf:*)\"]\n")
	path := filepath.Join(dir, "policies.yaml")
	mustWriteFile(t, path, "- {pattern: [npm, run], decision: allow}\n- {pattern: [rm], decision: forbidden}\n")
	before := "- {pattern: [npm, run], decision: allow}\n- {pattern: [rm], decision: forbidden}\n"
	out, err := runCLI(t, "lint", "--strict")
	if err != nil || len(findingLines(out, "LINT021")) != 0 {
		t.Errorf("matching policies lint: %v\n%s", err, out)
	}
	mustWriteFile(t, path, "- {pattern: [npm, run], decision: prompt}\n- {pattern: [rm], decision: forbidden}\n")
	out, err = runCLI(t, "lint", "--strict")
	if err == nil || !strings.Contains(out, "LINT021") || !strings.Contains(out, "matching prompt prefix") {
		t.Errorf("conflict should fail strict lint: %v\n%s", err, out)
	}
	mustWriteFile(t, path, before)
}

func TestLint_CodexPoliciesSkipDisabledTargetsAndFilteredSpecs(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\noutputs:\n  codex:\n    exec-policies: [{pattern: [git], decision: forbidden}]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/settings/security.yaml"), "targets: [claude]\npermissions:\n  allow: [\"Bash(git:*)\"]\n")
	out, err := runCLI(t, "lint")
	if err != nil || strings.Contains(out, "LINT021") {
		t.Errorf("disabled target lint: %v\n%s", err, out)
	}
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "targets: [claude, codex]\noutputs:\n  codex:\n    exec-policies: [{pattern: [git], decision: forbidden}]\n")
	out, err = runCLI(t, "lint")
	if err != nil || strings.Contains(out, "LINT021") {
		t.Errorf("Claude-only permissions lint: %v\n%s", err, out)
	}
}

func TestLint_CodexPoliciesRespectPortableRestrictions(t *testing.T) {
	budgetProject(t, `targets: [codex]
outputs:
  codex:
    exec-policies:
      - {pattern: [git], decision: allow}
      - {pattern: [git, push], decision: forbidden}
      - {pattern: [npm], decision: allow}
      - {pattern: [npm], decision: prompt}
  claude:
    settings:
      permissions:
        allow: ["Bash(git:*)", "Bash(npm:*)"]
        deny: ["Bash(git push:*)"]
        ask: ["Bash(npm:*)"]
`)
	out, err := runCLI(t, "lint", "--strict")
	if err != nil || strings.Contains(out, "LINT021") {
		t.Errorf("matching portable restrictions reported as drift: %v\n%s", err, out)
	}
}

func TestLint_CodexPoliciesCheckedOnlyForTranslatableRulesAndNamedBySource(t *testing.T) {
	invalid := "targets: [codex]\noutputs:\n  codex:\n    exec-policies: [{pattern: [git, diff], decision: allowed}]\n"
	dir := budgetProject(t, invalid)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\nallowed_tools: [Read]\n---\n\nReview.\n")
	out, err := runCLI(t, "lint")
	if err != nil || !strings.Contains(out, "LINT007") {
		t.Errorf("policy check without Bash permissions hid other findings: %v\n%s", err, out)
	}
	permissions := "  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\"]\n"
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n"+invalid+permissions)
	if _, err := runCLI(t, "lint"); err == nil || !strings.Contains(err.Error(), "agnostic-ai.yaml: exec-policies[0]") {
		t.Errorf("invalid inline policy should name agnostic-ai.yaml, got %v", err)
	}
	mustWriteFile(t, filepath.Join(dir, "policies.yaml"), "- {pattern: [git, diff], decision: allowed}\n")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    exec-policies: [{pattern: [git], decision: allow}]\n    exec-policies-file: policies.yaml\n"+permissions)
	if _, err := runCLI(t, "lint"); err == nil || !strings.Contains(err.Error(), "policies.yaml: exec-policies[0]") {
		t.Errorf("invalid file policy should name its file and index there, got %v", err)
	}
}

func TestLint_CodexPoliciesEmptyInlineListReportsMissingPermissions(t *testing.T) {
	budgetProject(t, "targets: [codex]\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n    exec-policies: []\n  claude:\n    settings:\n      permissions:\n        allow: [\"Bash(git diff:*)\"]\n")
	out, err := runCLI(t, "lint", "--strict")
	if err == nil || !strings.Contains(out, "LINT021") || !strings.Contains(out, "Bash(git diff:*) has no matching prefix") {
		t.Errorf("explicit empty list should report missing prefix: %v\n%s", err, out)
	}
}
