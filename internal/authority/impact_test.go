package authority

import (
	"slices"
	"strings"
	"testing"
)

func impactOf(t *testing.T, root string, opts Options, p Proposal) *ImpactResult {
	t.Helper()
	cfg, b := loadProject(t, root)
	if opts.Root == "" {
		opts.Root = root
	}
	opts.Now = fixedNow
	res, err := Impact(cfg, b, opts, p)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	return res
}

func TestImpact_RemovingAnAllowRuleLeavesTheUnmodeledDefault(t *testing.T) {
	res := impactOf(t, demoRoot, Options{}, Proposal{
		AgentID: "coding-agent",
		Target:  "claude",
		Changes: []Change{{Op: "remove", List: "allow", Rule: "mcp:railway/deploy"}},
	})

	if !res.Valid || res.Status != "proposed" || len(res.Problems) != 0 {
		t.Fatalf("result = valid %v, status %s, problems %v", res.Valid, res.Status, res.Problems)
	}
	if len(res.Changes) != 2 || res.Summary.Changed != 2 || res.Summary.Indeterminate != 2 {
		t.Fatalf("changes = %+v, summary %+v", res.Changes, res.Summary)
	}
	for _, c := range res.Changes {
		if c.OperationID != "mcp-railway-scope:deploy" || c.DeclaredBefore != EffectAllowed || c.DeclaredAfter != EffectDefault || c.EffectiveBefore != EffectAllowed || c.EffectiveAfter != EffectDefault {
			t.Errorf("change = %+v", c)
		}
		if c.Direction != "indeterminate" || !strings.Contains(c.Reason, "default for mcp:railway/deploy is not modeled") {
			t.Errorf("direction %s, reason %q; want indeterminate, naming the unknown default", c.Direction, c.Reason)
		}
	}
	if !slices.Equal(res.AffectedSubjects, []string{"session", "coding-agent"}) {
		t.Errorf("affected = %v, want the session and the one agent that can reach railway", res.AffectedSubjects)
	}
	if len(res.Diff) != 1 || res.Diff[0].Path != ".agnostic-ai/settings/permissions.yaml" {
		t.Fatalf("diff = %+v", res.Diff)
	}
	if !strings.Contains(res.Diff[0].Before, "- mcp:railway/deploy") || strings.Contains(res.Diff[0].After, "mcp:railway/deploy") {
		t.Errorf("diff does not remove the rule:\nbefore:\n%s\nafter:\n%s", res.Diff[0].Before, res.Diff[0].After)
	}
	if res.ProposedRevision == res.BaseRevision || res.ProposedRevision == "" {
		t.Errorf("revisions = %s → %s", res.BaseRevision, res.ProposedRevision)
	}
	if res.After == nil || res.After.Agent.ID != "coding-agent" || findOp(t, res.After, "mcp-railway-scope:deploy").Effect != EffectDefault {
		t.Error("after view does not show the proposed state for the selected agent")
	}
}

func TestImpact_AddingADenyRuleNarrowsEverySubjectThatCouldUseTheTool(t *testing.T) {
	change := []Change{{Op: "add", List: "deny", Rule: "mcp:github/create_pull_request"}}
	res := impactOf(t, demoRoot, Options{}, Proposal{Target: "claude", Changes: change})

	if res.Summary.Narrowed != 2 || res.Summary.Changed != 2 {
		t.Fatalf("summary = %+v, changes %+v", res.Summary, res.Changes)
	}
	for _, c := range res.Changes {
		if c.EffectiveBefore != EffectAllowed || c.EffectiveAfter != EffectDenied || c.Direction != "narrowed" || c.Reason != "" {
			t.Errorf("change = %+v", c)
		}
	}
	if slices.Contains(res.AffectedSubjects, "reviewer") {
		t.Error("the reviewer cannot create pull requests either way, so it is not affected")
	}

	codex := impactOf(t, demoRoot, Options{}, Proposal{Target: "codex", Changes: change})
	for _, c := range codex.Changes {
		if c.Direction != "indeterminate" || c.EffectiveBefore != EffectUnresolved || c.EffectiveAfter != EffectUnresolved {
			t.Errorf("codex change = %+v, want a declared-only change that stays unresolved", c)
		}
	}
	if len(codex.AffectedSubjects) != 0 {
		t.Errorf("codex affected = %v, want none: nothing effective changes", codex.AffectedSubjects)
	}
}

func TestImpact_InvalidChangesAreProblemsAndChangeNothing(t *testing.T) {
	res := impactOf(t, demoRoot, Options{}, Proposal{Changes: []Change{
		{Op: "add", List: "allow", Rule: "shell(go test"},
		{Op: "replace", List: "allow", Rule: "read"},
		{Op: "add", List: "maybe", Rule: "read"},
		{Op: "remove", List: "deny", Rule: "shell(curl:*)"},
		{Op: "add", List: "allow", Rule: "read", Source: "settings/missing.yaml"},
	}})

	if res.Valid || len(res.Problems) != 5 {
		t.Fatalf("valid %v, problems %v; want five problems", res.Valid, res.Problems)
	}
	if len(res.Changes) != 0 || len(res.Diff) != 0 || res.After != nil {
		t.Errorf("an invalid proposal reports changes %v, diff %v", res.Changes, res.Diff)
	}
	if !strings.Contains(res.Problems[3], `no deny rule "shell(curl:*)"`) {
		t.Errorf("problem = %q", res.Problems[3])
	}
}

func TestImpact_StaleRevisionIsRefused(t *testing.T) {
	res := impactOf(t, demoRoot, Options{}, Proposal{
		Revision: "000000000000",
		Changes:  []Change{{Op: "add", List: "deny", Rule: "shell(curl:*)"}},
	})

	if res.Valid || len(res.Problems) != 1 || !strings.Contains(res.Problems[0], "revision 000000000000") {
		t.Fatalf("valid %v, problems %v", res.Valid, res.Problems)
	}
	if len(res.Changes) != 0 || res.After != nil || res.BaseRevision == "" {
		t.Errorf("a stale proposal computed changes: %+v", res)
	}
}

func TestImpact_FirstRuleCreatesAPermissionsSpec(t *testing.T) {
	root := writeProject(t, map[string]string{"agnostic-ai.yaml": "version: 1\ntargets: [claude]\n"})
	res := impactOf(t, root, Options{}, Proposal{Changes: []Change{{Op: "add", List: "allow", Rule: "shell(go test:*)"}}})

	if !res.Valid || len(res.Diff) != 1 {
		t.Fatalf("valid %v, problems %v, diff %+v", res.Valid, res.Problems, res.Diff)
	}
	d := res.Diff[0]
	if d.Path != ".agnostic-ai/settings/permissions.yaml" || d.Before != "" || d.After != "permissions:\n  allow:\n    - shell(go test:*)\n" {
		t.Errorf("diff = %+v", d)
	}
	if len(res.Changes) != 1 || res.Changes[0].OperationID != "shell-go-test:run" || res.Changes[0].Direction != "indeterminate" {
		t.Errorf("changes = %+v, want the new command resource moving from the default", res.Changes)
	}
}

func TestJudgeDirection_RanksOnlyKnownDecisions(t *testing.T) {
	for _, tc := range []struct {
		before, after Effect
		direction     string
		reason        string
	}{
		{EffectAllowed, EffectAsk, "narrowed", ""},
		{EffectAsk, EffectAllowed, "widened", ""},
		{EffectAllowed, EffectUnavailable, "narrowed", ""},
		{EffectDenied, EffectUnavailable, "moved", ""},
		{EffectDefault, EffectAllowed, "indeterminate", "Before the change no rule applies, and the tool's default for shell is not modeled."},
		{EffectAllowed, EffectUnresolved, "indeterminate", "After the change the effect of shell on Codex is unresolved."},
		{EffectDenied, EffectDenied, "moved", "Only the declared decision changes; the effect on Codex stays denied."},
	} {
		direction, reason := judgeDirection(tc.before, tc.after, "shell", "Codex")
		if direction != tc.direction || reason != tc.reason {
			t.Errorf("%s → %s = %s %q, want %s %q", tc.before, tc.after, direction, reason, tc.direction, tc.reason)
		}
	}
}

func TestImpact_ProposedViewNeverInheritsInSyncCarriers(t *testing.T) {
	installed := map[string]InstalledState{"claude": {State: InstalledInSync, Files: []InstalledFile{
		{Path: ".claude/settings.json", State: InstalledInSync},
		{Path: "CLAUDE.md", State: InstalledInSync},
	}}}
	current := findOp(t, buildView(t, demoRoot, Options{Target: "claude", Installed: installed}), "mcp-github-scope:create_pull_request")
	if current.Installed != InstalledInSync || current.Hypothetical {
		t.Fatalf("current = installed %s, hypothetical %v; want in sync before the proposal", current.Installed, current.Hypothetical)
	}
	res := impactOf(t, demoRoot, Options{Installed: installed}, Proposal{Target: "claude", Changes: []Change{{Op: "add", List: "deny", Rule: "mcp:github/create_pull_request"}}})
	if res.After == nil {
		t.Fatal("no proposed view")
	}
	after := findOp(t, res.After, "mcp-github-scope:create_pull_request")
	if after.Effect != EffectDenied || after.Installed != InstalledStale || !after.Hypothetical {
		t.Errorf("proposed = effect %s, installed %s, hypothetical %v; want a denial that waits for a sync", after.Effect, after.Installed, after.Hypothetical)
	}
	if res.After.Harness.Installed.State != InstalledStale {
		t.Errorf("proposed harness installed = %s, want stale: the proposal writes nothing", res.After.Harness.Installed.State)
	}
	for _, f := range res.After.Harness.Installed.Files {
		if f.Path == "CLAUDE.md" && f.State != InstalledInSync {
			t.Errorf("CLAUDE.md = %s, want in-sync: the proposal does not touch it", f.State)
		}
	}
}
