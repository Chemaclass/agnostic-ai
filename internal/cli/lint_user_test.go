package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func runUserCheck(args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append(args, "--scope", "user"))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestLintUser_CleanHomeExitsZeroOutsideProject(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code\n---\nReview it.\n")
	writeFile(t, filepath.Join(source, "local", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	out, _, err := runUserCheck("lint", "--strict")
	if err != nil {
		t.Fatalf("lint --scope user on a clean home: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2 spec(s) clean") {
		t.Errorf("expected both layers counted as clean, got:\n%s", out)
	}
}

func TestLintUser_ReportsMistypedKeyInSharedLayer(t *testing.T) {
	_, source := userAgentTestHome(t)
	agent := filepath.Join(source, "agents", "reviewer.md")
	writeFile(t, agent, "---\nname: reviewer\ndescription: Review code\nmodl: opus\n---\nReview it.\n")

	out, _, err := runUserCheck("lint")
	if err != nil {
		t.Fatalf("a warning alone must not fail lint --scope user: %v", err)
	}
	if !strings.Contains(out, "LINT007") || !strings.Contains(out, agent) {
		t.Errorf("expected LINT007 on %s, got:\n%s", agent, out)
	}
	if _, _, err := runUserCheck("lint", "--strict"); err == nil {
		t.Error("lint --scope user --strict must fail on a warning")
	}
}

func TestLintUser_BrokenLocalOverrideExitsOne(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\n---\nShared.\n")
	override := filepath.Join(source, "local", "agents", "helper.md")
	writeFile(t, override, "---\nname: helper\ndescription: Help\nLocal body with no closing delimiter.\n")

	out, _, err := runUserCheck("lint")
	if err == nil {
		t.Fatalf("lint --scope user must exit non-zero on an error finding:\n%s", out)
	}
	if !strings.Contains(out, "LINT006") || !strings.Contains(out, override) {
		t.Errorf("expected LINT006 on the local override %s, got:\n%s", override, out)
	}
}

func TestValidateUser_CleanHomeExitsZeroOutsideProject(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "hooks", "start.yaml"), "name: start\nevent: SessionStart\ncommand: echo hi\n")
	writeFile(t, filepath.Join(source, "local", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	out, _, err := runUserCheck("validate")
	if err != nil {
		t.Fatalf("validate --scope user on a clean home: %v\n%s", err, out)
	}
	if !strings.Contains(out, "loaded 2 entries.") {
		t.Errorf("expected both layers loaded, got:\n%s", out)
	}
}

func TestValidateUser_ReportsUnknownHookEventAndScopedLocalRule(t *testing.T) {
	_, source := userAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	writeFile(t, hook, "name: start\nevent: OnBoot\ncommand: echo hi\n")
	rule := filepath.Join(source, "local", "rules", "go.md")
	writeFile(t, rule, "---\nname: go\nglobs: \"**/*.go\"\n---\nGo rule.\n")

	out, _, err := runUserCheck("validate")
	if err == nil {
		t.Fatalf("validate --scope user must exit non-zero on issues:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, `unknown hook event "OnBoot"`) {
		t.Errorf("expected the unknown hook event on %s, got:\n%s", hook, out)
	}
	if !strings.Contains(out, rule) || !strings.Contains(out, "unconditionally") {
		t.Errorf("expected the scoped local rule %s reported, got:\n%s", rule, out)
	}
}

func TestValidateUser_ReportsEventOnlyANonHookTargetKnows(t *testing.T) {
	_, source := userAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "save.yaml")
	writeFile(t, hook, "name: save\nevent: PostCompaction\ncommand: echo saved\n")

	out, _, err := runUserCheck("validate")
	if err == nil {
		t.Fatalf("validate --scope user must reject an event no hook-writing user-level target fires:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, `unknown hook event "PostCompaction"`) {
		t.Errorf("expected the Devin-only event reported on %s, got:\n%s", hook, out)
	}
}

func TestLintUser_ScopedRuleExitsOne(t *testing.T) {
	_, source := userAgentTestHome(t)
	rule := filepath.Join(source, "rules", "backend", "x.md")
	writeFile(t, rule, "---\nname: x\nglobs: \"**/*.go\"\n---\nGo rule.\n")

	out, _, err := runUserCheck("lint")
	if err == nil {
		t.Fatalf("lint --scope user must exit non-zero on a rule sync --scope user rejects:\n%s", out)
	}
	if !strings.Contains(out, "LINT010") || !strings.Contains(out, rule) {
		t.Errorf("expected LINT010 on %s, got:\n%s", rule, out)
	}
}

func TestLintUser_EmptyHintNamesResolvedSourceRoot(t *testing.T) {
	home, _ := userAgentTestHome(t)
	missing := filepath.Join(home, "agnostic-typo")
	t.Setenv("AGNOSTIC_AI_HOME", missing)

	_, errOut, err := runUserCheck("lint")
	if err != nil {
		t.Fatalf("an empty user source must exit 0 like sync --scope user: %v", err)
	}
	if !strings.Contains(errOut, missing) || strings.Contains(errOut, "$AGNOSTIC_AI_HOME") {
		t.Errorf("expected the hint to name %s, got:\n%s", missing, errOut)
	}
}
