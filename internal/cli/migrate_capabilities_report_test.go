package cli

import (
	"strings"
	"testing"
)

func TestMigrate_CapabilitiesNamesKeptAliasesUnderTheRewrite(t *testing.T) {
	migrationFixture(t, "capabilities-agent-tools")
	silence(t)

	out, err := runCLI(t, "migrate", "--only", "capabilities")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if strings.Contains(out, "skipped .agnostic-ai/agents/reviewer.md") {
		t.Errorf("a rewritten agent must not also read as skipped:\n%s", out)
	}
	want := "  rewrote .agnostic-ai/agents/reviewer.md\n    keeps Grep as an alias: no capability stands for it alone\n"
	if !strings.Contains(out, want) {
		t.Errorf("output misses %q:\n%s", want, out)
	}
}

func TestMigrate_CapabilitiesRerunSaysNothingForARewrittenSkill(t *testing.T) {
	migrationFixture(t, "capabilities-skill-tools")
	silence(t)

	if out, err := runCLI(t, "migrate", "--only", "capabilities"); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	out, err := runCLI(t, "migrate", "--only", "capabilities")
	if err != nil {
		t.Fatalf("second migrate: %v\n%s", err, out)
	}
	if strings.Contains(out, "review/SKILL.md") {
		t.Errorf("second run names review/SKILL.md, which the first run rewrote:\n%s", out)
	}
}

func TestMigrate_ListCountsOnlyFilesLeftAsWritten(t *testing.T) {
	migrationFixture(t, "capabilities-agent-tools")
	silence(t)

	out, _ := runCLI(t, "migrate", "--list")
	if !strings.Contains(out, "capabilities-agent-tools (0.79.0): rewrite an agent's tools: as can:, with neutral capability names: 2 to rewrite, 1 skipped") {
		t.Errorf("--list counts a rewritten file as skipped:\n%s", out)
	}
}
