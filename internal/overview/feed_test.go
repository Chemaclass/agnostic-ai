package overview

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func feedOf(t *testing.T, in Inputs) []Card {
	t.Helper()
	if in.Cfg == nil {
		in.Cfg = &config.Config{Targets: []string{"claude", "codex"}}
	}
	if len(in.Bundle.All()) == 0 {
		in.Bundle = handcraftedBundle()
	}
	return BuildFeed(in).Feed
}

func actionKinds(c Card) []string {
	var out []string
	for _, a := range c.Actions {
		out = append(out, a.Kind)
	}
	return out
}

func action(t *testing.T, c Card, id string) CardAction {
	t.Helper()
	for _, a := range c.Actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("card %q has no %q action: %+v", c.Title, id, c.Actions)
	return CardAction{}
}

func TestBuildFeed_LintFindingBecomesAHygieneCardWithTheExplainCommand(t *testing.T) {
	finding := Finding{
		Source:      SourceLint,
		Code:        "LINT001",
		Severity:    "warn",
		Message:     "rule testing has no body and no description. Write one, or delete the file",
		Path:        ".agnostic-ai/rules/testing.md",
		Explanation: "A spec has no body and no description, so every target receives an empty file or entry.",
	}
	feed := feedOf(t, Inputs{Snapshot: authority.Snapshot{Revision: "rev1"}, Findings: []Finding{finding}})
	if len(feed) != 1 {
		t.Fatalf("feed = %+v", feed)
	}
	c := feed[0]
	if c.Kind != CardInsight || c.Priority != PriorityHygiene || c.PriorityLabel != "Hygiene" || c.Source != SourceLint || c.Code != "LINT001" {
		t.Errorf("card = %+v", c)
	}
	if c.Title != "rule testing has no body and no description" {
		t.Errorf("title = %q, want the message's first sentence", c.Title)
	}
	if c.Consequence != finding.Explanation {
		t.Errorf("consequence = %q, want the lint explanation", c.Consequence)
	}
	if c.SectionID != SectionContext {
		t.Errorf("section = %q, want the rule's section", c.SectionID)
	}
	if !reflect.DeepEqual(c.Evidence, []CardEvidence{{Path: finding.Path, Detail: finding.Message}}) {
		t.Errorf("evidence = %+v", c.Evidence)
	}
	if got := actionKinds(c); !slices.Equal(got, []string{ActionCopyCommand, ActionOpen, ActionDismiss, ActionSnooze}) {
		t.Errorf("actions = %v", got)
	}
	if got := action(t, c, "explain").Command; got != "agnostic-ai explain LINT001" {
		t.Errorf("command = %q", got)
	}
	if got := action(t, c, "open").Href; got != "/?section=context" {
		t.Errorf("open href = %q", got)
	}
	if c.ID != "lint:"+c.Fingerprint || len(c.Fingerprint) != 12 || c.Revision != "rev1" {
		t.Errorf("id = %q, fingerprint = %q, revision = %q", c.ID, c.Fingerprint, c.Revision)
	}
}

func TestBuildFeed_LintWithoutAnExplanationNamesWhatItsSeverityFails(t *testing.T) {
	feed := feedOf(t, Inputs{Findings: []Finding{
		{Source: SourceLint, Code: "LINT999", Severity: "error", Message: "broken", Path: "agnostic-ai.yaml"},
		{Source: SourceLint, Severity: "warn", Message: "odd", Path: "agnostic-ai.yaml"},
	}})
	if len(feed) != 2 {
		t.Fatalf("feed = %+v", feed)
	}
	if feed[0].Title != "broken" || !strings.Contains(feed[0].Consequence, "lint and doctor fail") {
		t.Errorf("error card = %+v", feed[0])
	}
	if feed[1].Title != "odd" || !strings.Contains(feed[1].Consequence, "--strict") {
		t.Errorf("warning card = %+v", feed[1])
	}
	if got := actionKinds(feed[1]); slices.Contains(got, ActionCopyCommand) {
		t.Errorf("a finding without a code got a command: %v", got)
	}
}

func TestBuildFeed_PermissionLintBecomesASecurityCard(t *testing.T) {
	for _, code := range []string{"LINT038", "LINT009"} {
		feed := feedOf(t, Inputs{Findings: []Finding{{
			Source:  SourceLint,
			Code:    code,
			Message: "permissions.allow: bare read now covers every file read (claude: Read); use read(src/**) to name paths",
			Path:    ".agnostic-ai/settings/permissions.yaml",
		}}})
		if len(feed) != 1 {
			t.Fatalf("%s feed = %+v", code, feed)
		}
		c := feed[0]
		if c.Priority != PrioritySecurity || c.PriorityLabel != "Security" || c.Kind != CardInsight {
			t.Errorf("%s card = %+v", code, c)
		}
		if c.SectionID != SectionAuthority || action(t, c, "open").Href != "/authority" {
			t.Errorf("%s section = %q, open = %q", code, c.SectionID, action(t, c, "open").Href)
		}
		if c.Title != "permissions.allow: bare read now covers every file read (claude: Read)" {
			t.Errorf("%s title = %q", code, c.Title)
		}
		if action(t, c, "explain").Command != "agnostic-ai explain "+code {
			t.Errorf("%s command = %q", code, action(t, c, "explain").Command)
		}
	}
}

func TestBuildFeed_DoctorFindingOffersTheFixOnlyWhenFixable(t *testing.T) {
	feed := feedOf(t, Inputs{Findings: []Finding{
		{Source: SourceDoctor, Code: "nested-claude-copy", Message: "Nested CLAUDE.md repeats rule api, which sync already writes for Claude Code", Path: "src/api/CLAUDE.md", Fix: "agnostic-ai doctor --fix", Explanation: "Claude Code loads the same text twice.", Section: SectionContext},
		{Source: SourceDoctor, Code: "divergent-hook-script", Message: "Hook script fmt.sh differs between claude, codex; move the agreed body", Path: ".agnostic-ai/scripts/claude/fmt.sh", Section: SectionControl},
	}})
	if len(feed) != 2 {
		t.Fatalf("feed = %+v", feed)
	}
	byCode := map[string]Card{}
	for _, c := range feed {
		byCode[c.Code] = c
		if c.Source != SourceDoctor || c.Priority != PriorityHygiene || c.Kind != CardInsight {
			t.Errorf("card = %+v", c)
		}
	}
	fixable := byCode["nested-claude-copy"]
	if action(t, fixable, "fix").Command != "agnostic-ai doctor --fix" || fixable.Consequence != "Claude Code loads the same text twice." || fixable.SectionID != SectionContext {
		t.Errorf("fixable card = %+v", fixable)
	}
	manual := byCode["divergent-hook-script"]
	if slices.Contains(actionKinds(manual), ActionCopyCommand) || manual.SectionID != SectionControl {
		t.Errorf("manual card = %+v", manual)
	}
}

func TestBuildFeed_DriftBecomesOneDecisionCardPerTargetWithTheSyncCommand(t *testing.T) {
	feed := feedOf(t, Inputs{Findings: []Finding{
		{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
		{Source: SourceDrift, Target: "claude", Path: ".claude/settings.json", Message: "not generated"},
		{Source: SourceDrift, Target: "codex", Path: "AGENTS.md", Message: "stale"},
	}})
	if len(feed) != 2 {
		t.Fatalf("feed = %+v", feed)
	}
	var claude Card
	for _, c := range feed {
		if c.Kind != CardDecision || c.Priority != PriorityDrift || c.PriorityLabel != "Drift" || c.Source != SourceDrift || c.SectionID != SectionControl {
			t.Errorf("card = %+v", c)
		}
		if strings.HasPrefix(c.Title, "Claude Code") {
			claude = c
		}
	}
	if claude.Title != "Claude Code runs on stale generated configuration" {
		t.Fatalf("claude card = %+v", claude)
	}
	if claude.Consequence != "The files sync would write for Claude Code differ from what is on disk, so the tool is not using the current specs." {
		t.Errorf("consequence = %q", claude.Consequence)
	}
	want := []CardEvidence{{Path: ".claude/settings.json", Detail: "not generated"}, {Path: "CLAUDE.md", Detail: "stale"}}
	if !reflect.DeepEqual(claude.Evidence, want) {
		t.Errorf("evidence = %+v, want %+v", claude.Evidence, want)
	}
	if got := action(t, claude, "sync"); got.Kind != ActionCopyCommand || got.Command != "agnostic-ai sync -t claude" {
		t.Errorf("sync action = %+v", got)
	}
	if got := action(t, claude, "open").Href; got != "/?section=control" {
		t.Errorf("open href = %q", got)
	}
	if got := actionKinds(claude); !slices.Equal(got[len(got)-2:], []string{ActionDismiss, ActionSnooze}) {
		t.Errorf("actions = %v, want dismiss and snooze last", got)
	}
}

// authorityView is a session subject's view on claude with one
// expressible rule, a target that cannot express it, and an MCP server
// that declares no roots.
func authorityView() *authority.View {
	allowShell := authority.Grant{Rule: "shell", List: "allow", Scope: "project", Source: authority.Source{Path: ".agnostic-ai/settings/permissions.yaml", Layer: "project", Line: 3}, Wins: true}
	denyEnv := authority.Grant{Rule: "edit(.env)", List: "deny", Scope: "project", Source: authority.Source{Path: ".agnostic-ai/settings/permissions.yaml", Layer: "project", Line: 9}, Wins: true}
	return &authority.View{
		SchemaVersion: authority.SchemaVersion,
		Snapshot:      authority.Snapshot{Revision: "authrev", Targets: []string{"claude", "cursor"}},
		Harness:       authority.Harness{Target: "claude", Label: "Claude Code", Available: []authority.HarnessOption{{Target: "claude", Label: "Claude Code"}, {Target: "cursor", Label: "Cursor"}}},
		Agent:         authority.Subject{SubjectSummary: authority.SubjectSummary{ID: authority.SessionSubjectID, Label: "Session", Kind: authority.SubjectSession}},
		Domains:       []authority.Domain{{ID: authority.DomainFilesystem}, {ID: authority.DomainCloud}},
		Providers: []authority.Provider{
			{ID: "builtin:shell", Label: "Shell", Kind: "builtin", DomainIDs: []string{authority.DomainFilesystem}},
			{ID: "mcp:railway", Label: "Railway", Kind: "mcp", DomainIDs: []string{authority.DomainCloud}, Source: &authority.Source{Path: ".agnostic-ai/mcps/railway.yaml", Layer: "project"}},
		},
		Resources: []authority.Resource{
			{
				ID: "resource:shell:any", Label: "Any command", Kind: authority.ResourceCommandPattern, ProviderID: "builtin:shell",
				DomainIDs: []string{authority.DomainFilesystem}, Identity: authority.Identity{Pattern: "*"},
				Operations: []authority.Operation{{ID: "op:shell:any:run", Action: "run", Label: "Run", Declared: authority.EffectAllowed, Effect: authority.EffectAllowed, Resolution: authority.ResolutionTranslated, Decided: &allowShell, Grants: []authority.Grant{allowShell}}},
			},
			{
				ID: "resource:file:.env", Label: ".env", Kind: authority.ResourceFilePattern, ProviderID: "builtin:files",
				Operations: []authority.Operation{{ID: "op:file:.env:edit", Action: "edit", Label: "Edit", Declared: authority.EffectDenied, Effect: authority.EffectDenied, Decided: &denyEnv, Grants: []authority.Grant{denyEnv}}},
			},
			{ID: "resource:railway:scope", Label: "Railway (scope not declared)", Kind: authority.ResourceUndeclaredScope, ProviderID: "mcp:railway"},
		},
		Uncertainties: []authority.Uncertainty{
			{ID: "u1", Code: authority.UncertaintyTargetNotExpressed, Severity: "warning", Message: "Cursor writes no permission rule for edit(.env), so its defaults apply.", RelatedIDs: []string{"cursor", "op:file:.env:edit", authority.DomainFilesystem}},
			{ID: "u2", Code: authority.UncertaintyScopeUndeclared, Severity: "info", Message: "Railway names no roots; its credential's reach is not inspected.", RelatedIDs: []string{"mcp:railway"}},
			{ID: "u3", Code: authority.UncertaintyRuntimeNotObserved, Severity: "info", Message: "Runtime enforcement is not observed."},
		},
	}
}

func cardWithCode(feed []Card, code string) (Card, bool) {
	for _, c := range feed {
		if c.Code == code {
			return c, true
		}
	}
	return Card{}, false
}

func TestBuildFeed_AuthorityUncertaintiesBecomeCoverageAndInformationCards(t *testing.T) {
	feed := feedOf(t, Inputs{Authority: authorityView()})

	coverage, ok := cardWithCode(feed, authority.UncertaintyTargetNotExpressed)
	if !ok {
		t.Fatalf("no target-not-expressed card in %+v", feed)
	}
	if coverage.Priority != PriorityCoverage || coverage.PriorityLabel != "Coverage" || coverage.Kind != CardInsight || coverage.Source != SourceAuthority {
		t.Errorf("coverage card = %+v", coverage)
	}
	if coverage.Title != "Cursor receives no rule for 1 declared decision" {
		t.Errorf("title = %q", coverage.Title)
	}
	if coverage.SectionID != SectionAuthority || coverage.DomainID != authority.DomainFilesystem {
		t.Errorf("section = %q, domain = %q", coverage.SectionID, coverage.DomainID)
	}
	if got := action(t, coverage, "open").Href; got != "/authority?target=cursor" {
		t.Errorf("open href = %q", got)
	}
	if !reflect.DeepEqual(coverage.Evidence, []CardEvidence{{Path: ".agnostic-ai/settings/permissions.yaml", Line: 9, Detail: "deny rule edit(.env) on .env"}}) {
		t.Errorf("evidence = %+v", coverage.Evidence)
	}
	if coverage.Consequence != "Cursor writes no permission rule for edit(.env), so its defaults apply." {
		t.Errorf("consequence = %q", coverage.Consequence)
	}

	scope, ok := cardWithCode(feed, authority.UncertaintyScopeUndeclared)
	if !ok {
		t.Fatalf("no provider-scope-undeclared card in %+v", feed)
	}
	if scope.Priority != PriorityInfo || scope.PriorityLabel != "Information" || scope.Kind != CardInsight || scope.Title != "Railway declares no scope" {
		t.Errorf("scope card = %+v", scope)
	}
	if scope.DomainID != authority.DomainCloud || scope.SectionID != SectionAuthority {
		t.Errorf("domain = %q, section = %q", scope.DomainID, scope.SectionID)
	}
	if got := action(t, scope, "open").Href; got != "/authority?resource=resource%3Arailway%3Ascope&target=claude" {
		t.Errorf("open href = %q", got)
	}
	if !reflect.DeepEqual(scope.Evidence, []CardEvidence{{Path: ".agnostic-ai/mcps/railway.yaml", Detail: "Railway names no roots; its credential's reach is not inspected."}}) {
		t.Errorf("evidence = %+v", scope.Evidence)
	}

	if _, ok := cardWithCode(feed, authority.UncertaintyRuntimeNotObserved); ok {
		t.Errorf("an uncertainty with no card rule produced a card")
	}
}

func TestBuildFeed_AllowedAnyShellCommandBecomesASecurityDecisionWithARemoveProposal(t *testing.T) {
	feed := feedOf(t, Inputs{Authority: authorityView()})
	c, ok := cardWithCode(feed, shellAnyCode)
	if !ok {
		t.Fatalf("no shell card in %+v", feed)
	}
	if c.Kind != CardDecision || c.Priority != PrioritySecurity || c.Title != "Any shell command is permitted without asking" {
		t.Errorf("card = %+v", c)
	}
	if c.SectionID != SectionAuthority || c.DomainID != authority.DomainFilesystem || c.Source != SourceAuthority {
		t.Errorf("section = %q, domain = %q, source = %q", c.SectionID, c.DomainID, c.Source)
	}
	if feed[0].ID != c.ID {
		t.Errorf("first card = %q, want the security card first", feed[0].Title)
	}
	propose := action(t, c, "propose")
	want := &authority.Proposal{
		AgentID:  authority.SessionSubjectID,
		Target:   "claude",
		Revision: "authrev",
		Changes:  []authority.Change{{Op: "remove", List: "allow", Rule: "shell", Source: ".agnostic-ai/settings/permissions.yaml"}},
	}
	if propose.Kind != ActionPropose || !reflect.DeepEqual(propose.Proposal, want) {
		t.Errorf("propose = %+v, proposal = %+v", propose, propose.Proposal)
	}
	if got := action(t, c, "open").Href; got != "/authority?operation=op%3Ashell%3Aany%3Arun&resource=resource%3Ashell%3Aany&target=claude" {
		t.Errorf("open href = %q", got)
	}
	if got := actionKinds(c); !slices.Equal(got, []string{ActionPropose, ActionOpen, ActionDismiss, ActionSnooze}) {
		t.Errorf("actions = %v", got)
	}
	if !reflect.DeepEqual(c.Evidence, []CardEvidence{{Path: ".agnostic-ai/settings/permissions.yaml", Line: 3, Detail: "permissions.allow: shell"}}) {
		t.Errorf("evidence = %+v", c.Evidence)
	}
}

func TestBuildFeed_AnyShellCommandThatAsksOrBelongsToAnAgentRaisesNoCard(t *testing.T) {
	asks := authorityView()
	asks.Resources[0].Operations[0].Effect = authority.EffectAsk
	if _, ok := cardWithCode(feedOf(t, Inputs{Authority: asks}), shellAnyCode); ok {
		t.Errorf("an operation that asks raised the shell card")
	}
	declaredOnly := authorityView()
	declaredOnly.Resources[0].Operations[0].Effect = authority.EffectUnresolved
	if _, ok := cardWithCode(feedOf(t, Inputs{Authority: declaredOnly}), shellAnyCode); ok {
		t.Errorf("a declared allow the harness does not resolve raised the shell card")
	}
	agent := authorityView()
	agent.Agent = authority.Subject{SubjectSummary: authority.SubjectSummary{ID: "coding-agent", Kind: authority.SubjectAgent}}
	if _, ok := cardWithCode(feedOf(t, Inputs{Authority: agent}), shellAnyCode); ok {
		t.Errorf("an agent's view raised the session's shell card")
	}
}

func TestBuildFeed_SortsByPriorityThenTitleAndCapsAtFifty(t *testing.T) {
	var findings []Finding
	for i := range 60 {
		findings = append(findings, Finding{Source: SourceLint, Code: "LINT001", Message: fmt.Sprintf("finding %02d", 59-i), Path: fmt.Sprintf(".agnostic-ai/rules/r%02d.md", i)})
	}
	findings = append(findings,
		Finding{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
		Finding{Source: SourceLint, Code: "LINT038", Message: "permissions.allow: bare shell now covers every shell command", Path: ".agnostic-ai/settings/permissions.yaml"},
	)
	feed := feedOf(t, Inputs{Findings: findings})
	if len(feed) != maxCards {
		t.Fatalf("feed has %d cards, want %d", len(feed), maxCards)
	}
	if feed[0].Priority != PrioritySecurity || feed[1].Priority != PriorityDrift {
		t.Errorf("first cards = %q, %q", feed[0].Title, feed[1].Title)
	}
	for i := 1; i < len(feed); i++ {
		a, b := feed[i-1], feed[i]
		if a.Priority > b.Priority || (a.Priority == b.Priority && a.Title > b.Title) {
			t.Errorf("card %d (%d %q) sorts before card %d (%d %q)", i-1, a.Priority, a.Title, i, b.Priority, b.Title)
		}
	}
	if feed[2].Title != "finding 00" || feed[len(feed)-1].Title != "finding 47" {
		t.Errorf("hygiene cards run %q to %q, want finding 00 to finding 47", feed[2].Title, feed[len(feed)-1].Title)
	}
}

func TestBuildFeed_FingerprintsStayStableAcrossRevisions(t *testing.T) {
	findings := []Finding{
		{Source: SourceLint, Code: "LINT001", Message: "empty", Path: ".agnostic-ai/rules/testing.md"},
		{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
	}
	before := feedOf(t, Inputs{Snapshot: authority.Snapshot{Revision: "aaaaaaaaaaaa"}, Findings: findings, Authority: authorityView()})
	after := feedOf(t, Inputs{Snapshot: authority.Snapshot{Revision: "bbbbbbbbbbbb"}, Findings: findings, Authority: authorityView()})
	if len(before) != len(after) || len(before) == 0 {
		t.Fatalf("feeds differ in length: %d, %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Fingerprint != after[i].Fingerprint || before[i].ID != after[i].ID {
			t.Errorf("card %q changed fingerprint across revisions: %s, %s", before[i].Title, before[i].Fingerprint, after[i].Fingerprint)
		}
		if before[i].Revision != "aaaaaaaaaaaa" || after[i].Revision != "bbbbbbbbbbbb" {
			t.Errorf("card %q revisions = %q, %q", before[i].Title, before[i].Revision, after[i].Revision)
		}
	}

	changed := slices.Clone(findings)
	changed[0].Message = "empty, still"
	again := feedOf(t, Inputs{Findings: changed})
	lint, _ := cardWithCode(again, "LINT001")
	original, _ := cardWithCode(before, "LINT001")
	if lint.Fingerprint == original.Fingerprint {
		t.Errorf("a changed finding kept fingerprint %s", lint.Fingerprint)
	}
}

func TestBuildFeed_PlacesEachFindingInTheSectionOfItsSpecOrPath(t *testing.T) {
	where := newPlacer(handcraftedBundle())
	for _, tc := range []struct {
		finding Finding
		want    string
	}{
		{Finding{Path: ".agnostic-ai/agents/reviewer.md"}, SectionControl},
		{Finding{Path: ".agnostic-ai/mcps/github.yaml"}, SectionTools},
		{Finding{Path: ".agnostic-ai/reviews/checklist.md"}, SectionVerification},
		{Finding{Path: ".agnostic-ai/settings/tiers.yaml"}, SectionControl},
		{Finding{Path: ".agnostic-ai/settings/protected.yaml"}, SectionAuthority},
		{Finding{Path: ".agnostic-ai/settings/tiers.yaml", Message: "permissions.allow: use read instead of Read"}, SectionAuthority},
		{Finding{Path: ".agnostic-ai/memory/MEMORY.md"}, SectionMemory},
		{Finding{Path: "agnostic-ai.yaml", Message: "models.fast has no codex model"}, SectionControl},
		{Finding{Path: ".agnostic-ai/AGNOSTIC_AI.md"}, SectionContext},
		{Finding{Path: "src/CLAUDE.md", Section: SectionContext}, SectionContext},
		{Finding{Path: ".codex/hooks.json", Section: SectionControl}, SectionControl},
		{Finding{Path: ".codex/hooks.json", Section: "nowhere"}, SectionContext},
	} {
		if got := where.section(tc.finding); got != tc.want {
			t.Errorf("section(%+v) = %q, want %q", tc.finding, got, tc.want)
		}
	}
}

func TestBuildFeed_RepeatedFindingsMakeOneCard(t *testing.T) {
	f := Finding{Source: SourceLint, Code: "LINT001", Message: "empty", Path: ".agnostic-ai/rules/testing.md"}
	if feed := feedOf(t, Inputs{Findings: []Finding{f, f}}); len(feed) != 1 {
		t.Errorf("feed = %+v, want one card", feed)
	}
}

func TestFirstSentence_EndsAtTheFirstBreak(t *testing.T) {
	for in, want := range map[string]string{
		"One. Two.":                           "One",
		"permissions.allow: bare read; use x": "permissions.allow: bare read",
		"No break here.":                      "No break here",
		"Is it? Yes":                          "Is it",
		"  padded  ":                          "padded",
	} {
		if got := firstSentence(in); got != want {
			t.Errorf("firstSentence(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildFeed_HypotheticalShellCardStaysConditional(t *testing.T) {
	v := authorityView()
	v.Resources[0].Operations[0].Hypothetical = true
	v.Resources[0].Operations[0].Installed = authority.InstalledStale
	c, ok := cardWithCode(feedOf(t, Inputs{Authority: v}), shellAnyCode)
	if !ok {
		t.Fatal("no shell card for a hypothetical allow")
	}
	if c.Title != "Any shell command would be permitted without asking once synced" {
		t.Errorf("title = %q", c.Title)
	}
	if !strings.Contains(c.Consequence, "Once the generated permission file is synced (it is stale)") || strings.Contains(c.Consequence, " runs ") {
		t.Errorf("consequence = %q, want a conditional statement that claims no execution", c.Consequence)
	}
	synced, _ := cardWithCode(feedOf(t, Inputs{Authority: authorityView()}), shellAnyCode)
	if strings.Contains(synced.Consequence, " runs ") || !strings.Contains(synced.Consequence, "permission rules allow") {
		t.Errorf("in-sync consequence = %q, want what the rules allow, not what ran", synced.Consequence)
	}
}

func TestBuildFeed_RedactedWinningRuleOffersNoRemoveProposal(t *testing.T) {
	v := authorityView()
	op := &v.Resources[0].Operations[0]
	op.Decided.Rule, op.Decided.Redacted = "shell(<redacted>)", true
	op.Grants = []authority.Grant{*op.Decided}
	c, ok := cardWithCode(feedOf(t, Inputs{Authority: v}), shellAnyCode)
	if !ok {
		t.Fatal("no shell card")
	}
	if got := actionKinds(c); slices.Contains(got, ActionPropose) {
		t.Errorf("actions = %v, want no proposal for a redacted rule", got)
	}
	if len(c.Evidence) != 1 || !strings.Contains(c.Evidence[0].Detail, "holds a credential; edit the file directly") {
		t.Errorf("evidence = %+v", c.Evidence)
	}
}
