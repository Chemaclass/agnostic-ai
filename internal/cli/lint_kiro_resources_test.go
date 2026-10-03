package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

const kiroHelperAgent = "---\nname: helper\ndescription: Helps.\nx-kiro:\n  resources: [%s]\n---\nHelp.\n"

func kiroResourcesProject(t *testing.T, targets, resources string) {
	t.Helper()
	dir := budgetProject(t, targets)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md"), "Always body.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"), strings.Replace(kiroHelperAgent, "%s", resources, 1))
}

func TestLint_KiroAgentResourcesWithoutAGENTSMdWarns(t *testing.T) {
	kiroResourcesProject(t, "targets: [codex, kiro]\n", `"file://.kiro/steering/**"`)

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("a warning alone must pass: %v\n%s", err, out)
	}
	lines := findingLines(out, "LINT029")
	if len(lines) != 1 || !strings.Contains(lines[0], "helper") || !strings.Contains(lines[0], "file://AGENTS.md") || !strings.Contains(lines[0], "chat.disableInheritingDefaultResources") {
		t.Errorf("want one LINT029 naming the agent, the setting, and file://AGENTS.md:\n%s", out)
	}
}

func TestLint_KiroAgentResourcesListingAGENTSMdIsClean(t *testing.T) {
	kiroResourcesProject(t, "targets: [codex, kiro]\n", `"file://.kiro/steering/**", "file://AGENTS.md"`)

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT029")) != 0 {
		t.Errorf("listing file://AGENTS.md must clear the warning:\n%s", out)
	}
}

func TestLint_KiroAgentResourcesWithoutInlinedRulesIsClean(t *testing.T) {
	kiroResourcesProject(t, "targets: [kiro]\n", `"file://.kiro/steering/**"`)

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT029")) != 0 {
		t.Errorf("without an inlining target the rules stay in steering, so nothing is lost:\n%s", out)
	}
}

func TestNamesAgentsMd_MatchesResourcesThatLoadTheRootFile(t *testing.T) {
	for resource, want := range map[string]bool{
		"file://AGENTS.md":         true,
		"file://./AGENTS.md":       true,
		"file://*.md":              true,
		"file://**/AGENTS.md":      true,
		"file://**":                true,
		"file://docs/AGENTS.md":    false,
		"file://.kiro/steering/**": false,
		"AGENTS.md":                false,
	} {
		if got := namesAgentsMd(resource); got != want {
			t.Errorf("namesAgentsMd(%q) = %v, want %v", resource, got, want)
		}
	}
}

func TestLint_KiroAgentResourcesWithOnlyScopedRulesIsClean(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, kiro]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nglobs: [\"**/*.go\"]\n---\nGo body.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"), strings.Replace(kiroHelperAgent, "%s", `"file://.kiro/steering/**"`, 1))

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT029")) != 0 {
		t.Errorf("fileMatch rules keep their steering files, so nothing is lost:\n%s", out)
	}
}
