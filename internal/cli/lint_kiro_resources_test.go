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
