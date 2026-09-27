package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func runGlobalCheck(args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append(args, "--global"))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestLintGlobal_CleanHomeExitsZeroOutsideProject(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code\n---\nReview it.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	out, _, err := runGlobalCheck("lint", "--strict")
	if err != nil {
		t.Fatalf("lint --global on a clean home: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2 spec(s) clean") {
		t.Errorf("expected both layers counted as clean, got:\n%s", out)
	}
}

func TestLintGlobal_ReportsMistypedKeyInSharedLayer(t *testing.T) {
	_, source := globalAgentTestHome(t)
	agent := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, agent, "---\nname: reviewer\ndescription: Review code\nmodl: opus\n---\nReview it.\n")

	out, _, err := runGlobalCheck("lint")
	if err != nil {
		t.Fatalf("a warning alone must not fail lint --global: %v", err)
	}
	if !strings.Contains(out, "LINT007") || !strings.Contains(out, agent) {
		t.Errorf("expected LINT007 on %s, got:\n%s", agent, out)
	}
	if _, _, err := runGlobalCheck("lint", "--strict"); err == nil {
		t.Error("lint --global --strict must fail on a warning")
	}
}

func TestLintGlobal_BrokenLocalOverrideExitsOne(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\n---\nShared.\n")
	override := filepath.Join(source, "local", "agents", "helper.md")
	mustWriteGlobalTest(t, override, "---\nname: helper\ndescription: Help\nLocal body with no closing delimiter.\n")

	out, _, err := runGlobalCheck("lint")
	if err == nil {
		t.Fatalf("lint --global must exit non-zero on an error finding:\n%s", out)
	}
	if !strings.Contains(out, "LINT006") || !strings.Contains(out, override) {
		t.Errorf("expected LINT006 on the local override %s, got:\n%s", override, out)
	}
}

func TestValidateGlobal_CleanHomeExitsZeroOutsideProject(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "start.yaml"), "name: start\nevent: SessionStart\ncommand: echo hi\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	out, _, err := runGlobalCheck("validate")
	if err != nil {
		t.Fatalf("validate --global on a clean home: %v\n%s", err, out)
	}
	if !strings.Contains(out, "loaded 2 entries.") {
		t.Errorf("expected both layers loaded, got:\n%s", out)
	}
}

func TestValidateGlobal_ReportsUnknownHookEventAndScopedLocalRule(t *testing.T) {
	_, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	mustWriteGlobalTest(t, hook, "name: start\nevent: OnBoot\ncommand: echo hi\n")
	rule := filepath.Join(source, "local", "rules", "go.md")
	mustWriteGlobalTest(t, rule, "---\nname: go\nglobs: \"**/*.go\"\n---\nGo rule.\n")

	out, _, err := runGlobalCheck("validate")
	if err == nil {
		t.Fatalf("validate --global must exit non-zero on issues:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, `unknown hook event "OnBoot"`) {
		t.Errorf("expected the unknown hook event on %s, got:\n%s", hook, out)
	}
	if !strings.Contains(out, rule) || !strings.Contains(out, "unconditionally") {
		t.Errorf("expected the scoped local rule %s reported, got:\n%s", rule, out)
	}
}

func TestValidateGlobal_ReportsEventOnlyANonHookTargetKnows(t *testing.T) {
	_, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "save.yaml")
	mustWriteGlobalTest(t, hook, "name: save\nevent: PostFileSave\ncommand: echo saved\n")

	out, _, err := runGlobalCheck("validate")
	if err == nil {
		t.Fatalf("validate --global must reject an event no hook-writing global target fires:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, `unknown hook event "PostFileSave"`) {
		t.Errorf("expected the kiro-only event reported on %s, got:\n%s", hook, out)
	}
}

func TestLintGlobal_ScopedRuleExitsOne(t *testing.T) {
	_, source := globalAgentTestHome(t)
	rule := filepath.Join(source, "rules", "backend", "x.md")
	mustWriteGlobalTest(t, rule, "---\nname: x\nglobs: \"**/*.go\"\n---\nGo rule.\n")

	out, _, err := runGlobalCheck("lint")
	if err == nil {
		t.Fatalf("lint --global must exit non-zero on a rule sync --global rejects:\n%s", out)
	}
	if !strings.Contains(out, "LINT010") || !strings.Contains(out, rule) {
		t.Errorf("expected LINT010 on %s, got:\n%s", rule, out)
	}
}

func TestLintGlobal_EmptyHintNamesResolvedSourceRoot(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	missing := filepath.Join(home, "agnostic-typo")
	t.Setenv("AGNOSTIC_AI_HOME", missing)

	_, errOut, err := runGlobalCheck("lint")
	if err != nil {
		t.Fatalf("an empty global home must exit 0 like sync --global: %v", err)
	}
	if !strings.Contains(errOut, missing) || strings.Contains(errOut, "$AGNOSTIC_AI_HOME") {
		t.Errorf("expected the hint to name %s, got:\n%s", missing, errOut)
	}
}
