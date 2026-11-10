package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

const (
	alwaysRuleSpec = "---\nname: always\n---\nAlways body.\n"
	goRuleSpec     = "---\nname: go\nalwaysApply: false\nglobs: \"**/*.go\"\n---\nGo body.\n"
)

func ruleFilesProject(t *testing.T, config string) string {
	t.Helper()
	dir := budgetProject(t, config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md"), alwaysRuleSpec)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), goRuleSpec)
	return dir
}

func mustSync(t *testing.T, args ...string) {
	t.Helper()
	if out, err := runCLI(t, append([]string{"sync", "--gitignore=off"}, args...)...); err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, out)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Codex inlines every unscoped rule into the shared AGENTS.md, and Cline
// reads that file too, so .clinerules/ keeps only the rule AGENTS.md
// cannot scope (#1224).
func TestSync_ClineSkipsAlwaysOnRulesCodexInlinesIntoAGENTSMd(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\n")

	mustSync(t)

	if n := strings.Count(readFile(t, filepath.Join(dir, "AGENTS.md")), "Always body."); n != 1 {
		t.Errorf("AGENTS.md should carry the always-on rule once, got %d", n)
	}
	if exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error(".clinerules/always.md loads the rule a second time next to AGENTS.md")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".clinerules", "go.md")), "Go body.") {
		t.Error(".clinerules/go.md must stay: only the rules folder scopes it by path")
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
}

func TestSync_AddingCodexSweepsTheClineCopyOfAnAlwaysOnRule(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [cline]\n")
	mustSync(t)
	if !exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Fatal("without codex, .clinerules/always.md is the only copy Cline gets")
	}

	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, cline]\n")
	mustSync(t)

	if exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error("sync must sweep the .clinerules copy it no longer writes")
	}
}

// A partial sync renders the shared AGENTS.md for every configured
// reader, so it keeps codex's rules block, and Cline still sees the
// rule once.
func TestSync_PartialSyncKeepsInlinedRulesForConfiguredReaders(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\n")
	mustSync(t)

	mustSync(t, "--only", "cline")

	if !strings.Contains(readFile(t, filepath.Join(dir, "AGENTS.md")), "Always body.") {
		t.Error("sync --only cline dropped codex's inlined rules from the shared AGENTS.md")
	}
	if exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error("sync --only cline wrote the rule a second time into .clinerules/")
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after a partial sync: %v\n%s", err, out)
	}
}

// Trae reads AGENTS.md only once its "Include AGENTS.md in the context"
// switch is on, so its own rules folder keeps every rule.
func TestSync_TraeKeepsAlwaysOnRulesBesideAGENTSMd(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, trae]\n")

	mustSync(t)

	if !exists(filepath.Join(dir, ".trae", "rules", "always.md")) {
		t.Error(".trae/rules/always.md must stay: Trae may not read AGENTS.md")
	}
}

// A user-owned AGENTS.md never receives the rules block, so Cline's
// rules folder must keep the rule.
func TestSync_ClineKeepsAlwaysOnRulesWhenAGENTSMdIsUnmanaged(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\nsync:\n  unmanaged: [AGENTS.md]\n")

	mustSync(t)

	if !exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error(".clinerules/always.md must stay when sync does not write AGENTS.md")
	}
}

// An outputs.cline.file override moves Cline off the shared AGENTS.md.
func TestSync_ClineKeepsAlwaysOnRulesWithItsOwnEntryPoint(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\noutputs:\n  cline:\n    file: CLINE.md\n")

	mustSync(t)

	if !exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error(".clinerules/always.md must stay when Cline reads its own entry point")
	}
}

func TestRender_ClineShowsTheAGENTSMdSectionForAnInlinedRule(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cline]\n")

	out, err := runCLI(t, "render", filepath.Join(".agnostic-ai", "rules", "always.md"), "-t", "cline")
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}
	var header string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# target: cline") {
			header = line
		}
	}
	if !strings.HasSuffix(header, " AGENTS.md") || !strings.Contains(out, "Always body.") {
		t.Errorf("render should show the AGENTS.md section Cline reads, got:\n%s", out)
	}
	if strings.Contains(out, ".clinerules") {
		t.Errorf("render shows a .clinerules file sync no longer writes:\n%s", out)
	}
}

func TestLintBudget_DoesNotCountRuleFilesAGENTSMdAlreadyCarries(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cline]\nlint:\n  instructions-words: 1\n")

	out, _ := runCLI(t, "lint")

	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "cline") && strings.Contains(line, "always-on rule files") {
			t.Errorf("Cline loads the always-on rule from AGENTS.md only, got:\n%s", line)
		}
	}
}

func TestExplain_CreditsAGENTSMdToClineForAnInlinedRule(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cline]\n")

	out, err := runCLI(t, "explain", filepath.Join(".agnostic-ai", "rules", "always.md"))
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	if !strings.Contains(out, `[cline] AGENTS.md (section "always")`) {
		t.Errorf("explain should credit the AGENTS.md section to cline, got:\n%s", out)
	}
	if strings.Contains(out, ".clinerules/always.md") {
		t.Errorf("explain lists a .clinerules file sync no longer writes:\n%s", out)
	}
}

// AGENTS.md holds codex's view of a rule: fences resolved for codex and
// variables left as written. A reader whose own copy differs keeps it,
// or it would lose the text that is only in that copy.
func TestSync_ClineKeepsARuleWhoseBodyDiffersFromTheInlinedOne(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "fenced.md"),
		"---\nname: fenced\n---\nShared line.\n\n::target cline\nCLINE ONLY LINE.\n::end\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "vars.md"),
		"---\nname: vars\n---\nSkills live in {{$SKILLS_DIR}}.\n")

	mustSync(t)

	if !strings.Contains(readFile(t, filepath.Join(dir, ".clinerules", "fenced.md")), "CLINE ONLY LINE.") {
		t.Error("the cline-only fence reaches Cline only through .clinerules/fenced.md")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".clinerules", "vars.md")), "Skills live in .cline/skills.") {
		t.Error("the expanded variable reaches Cline only through .clinerules/vars.md")
	}
}

// Kilo and Augment read only the root AGENTS.md, so a moved entry point
// carries rules neither loads.
func TestSync_KiloAndAugmentKeepRuleFilesWithAMovedEntryPoint(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [kilo, augment]\noutputs:\n  kilo:\n    file: docs/KILO.md\n  augment:\n    file: docs/AUG.md\n")

	mustSync(t)

	for _, f := range []string{".kilo/rules/always.md", ".augment/rules/always.md"} {
		if !exists(filepath.Join(dir, f)) {
			t.Errorf("%s must stay: the rule reaches no root AGENTS.md", f)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "kilo.jsonc")), ".kilo/rules/always.md") {
		t.Error("kilo.jsonc must list the rule file")
	}
}

func TestSync_ClineKeepsRuleFilesWhenItSharesAMovedEntryPointWithCodex(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\noutputs:\n  codex:\n    file: docs/AGENTS.md\n  cline:\n    file: docs/AGENTS.md\n")

	mustSync(t)

	if !exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error(".clinerules/always.md must stay: Cline reads the root AGENTS.md, not docs/AGENTS.md")
	}
}

// Kilo loads .kilo/rules only through `instructions`, so an entry the
// user wrote there must survive; only the entries for rules AGENTS.md
// now carries go.
func TestSync_KiloDropsOnlyTheInstructionsEntriesOfInlinedRules(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [kilo]\n")
	mustWriteFile(t, filepath.Join(dir, ".kilo", "rules", "mine.md"), "My own rule.\n")
	mustWriteFile(t, filepath.Join(dir, "kilo.jsonc"),
		`{"instructions": [".kilo/rules/always.md", ".kilo/rules/go.md", ".kilo/rules/mine.md", "docs/x.md"]}`+"\n")

	mustSync(t)

	got := readFile(t, filepath.Join(dir, "kilo.jsonc"))
	for _, gone := range []string{".kilo/rules/always.md", ".kilo/rules/go.md"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s points at a rule AGENTS.md carries now:\n%s", gone, got)
		}
	}
	for _, kept := range []string{".kilo/rules/mine.md", "docs/x.md"} {
		if !strings.Contains(got, kept) {
			t.Errorf("the user's own entry %s was dropped:\n%s", kept, got)
		}
	}
}

// A copy a previous sync wrote and this one no longer writes still loads
// until a full sync removes it, so check must not pass while it is there.
func TestSyncCheck_ReportsARuleFileTheNextSyncRemoves(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [cline]\n")
	mustSync(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, cline]\n")
	mustSync(t, "--only", "codex")

	out, err := runCLI(t, "sync", "--check", "--json")
	if err == nil || !strings.Contains(out, `".clinerules/always.md"`) || !strings.Contains(out, `"leftover"`) {
		t.Errorf("sync --check must name the copy a full sync removes, got %v:\n%s", err, out)
	}
	if out, _ := runCLI(t, "status"); strings.Contains(out, "in sync") {
		t.Errorf("status must not say in sync while the copy remains:\n%s", out)
	}

	mustSync(t)
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after a full sync: %v\n%s", err, out)
	}
}

func TestImport_ClineReadsTheRulesBlockItsCopiesWereSkippedFor(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cline]\n")
	mustSync(t)
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "rules")); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "import", "cline"); err != nil {
		t.Fatalf("import cline: %v\n%s", err, out)
	}

	if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md")), "Always body.") {
		t.Error("import cline must recover the rule Cline reads from AGENTS.md")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md")), "**/*.go") {
		t.Error("import cline must keep the scoped rule from .clinerules/go.md")
	}
}

// Kiro custom agents inherit AGENTS.md and steering alongside their own
// `resources` by default, so a steering copy would load each rule twice.
func TestSync_KiroSkipsSteeringCopyWhenAnAgentSetsResources(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, kiro]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"),
		"---\nname: helper\ndescription: Helps.\nx-kiro:\n  resources: [\"file://.kiro/steering/**\"]\n---\nHelp.\n")

	mustSync(t)

	if exists(filepath.Join(dir, ".kiro", "steering", "always.md")) {
		t.Error(".kiro/steering/always.md must not be written: AGENTS.md already carries the rule")
	}
	if n := strings.Count(readFile(t, filepath.Join(dir, "AGENTS.md")), "Always body."); n != 1 {
		t.Errorf("AGENTS.md carries the rule %d times, want 1", n)
	}
}

// Devin reads AGENTS.md as always-on rules next to .devin/rules/, so
// Windsurf leaves the always-on rule to AGENTS.md (#3230).
func TestSync_WindsurfSkipsAlwaysOnRulesCodexInlinesIntoAGENTSMd(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, windsurf]\n")

	mustSync(t)

	if exists(filepath.Join(dir, ".devin", "rules", "always.md")) {
		t.Error(".devin/rules/always.md loads the rule a second time next to AGENTS.md")
	}
	if !exists(filepath.Join(dir, ".devin", "rules", "go.md")) {
		t.Error(".devin/rules/go.md must stay: only the rules folder scopes it by path")
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
}

// Cursor applies the root AGENTS.md as rules alongside .cursor/rules/,
// so the always-on rule loads from AGENTS.md alone (#3230).
func TestSync_CursorSkipsAlwaysOnRulesCodexInlinesIntoAGENTSMd(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cursor]\n")

	mustSync(t)

	if exists(filepath.Join(dir, ".cursor", "rules", "always.mdc")) {
		t.Error(".cursor/rules/always.mdc loads the rule a second time next to AGENTS.md")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".cursor", "rules", "go.mdc")), "Go body.") {
		t.Error(".cursor/rules/go.mdc must stay: only the rules folder scopes it by path")
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
}

func TestSync_CursorKeepsAlwaysOnRulesWithoutARulesBlockInAGENTSMd(t *testing.T) {
	for _, targets := range []string{"[cursor]", "[cursor, windsurf, trae]", "[claude, cursor]"} {
		t.Run(targets, func(t *testing.T) {
			dir := ruleFilesProject(t, "targets: "+targets+"\n")

			mustSync(t)

			if !exists(filepath.Join(dir, ".cursor", "rules", "always.mdc")) {
				t.Error(".cursor/rules/always.mdc must stay: no AGENTS.md carries the rule")
			}
		})
	}
}

func TestSync_CursorKeepsAlwaysOnRulesWhenAGENTSMdIsUnmanaged(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cursor]\nsync:\n  unmanaged: [AGENTS.md]\n")

	mustSync(t)

	if !exists(filepath.Join(dir, ".cursor", "rules", "always.mdc")) {
		t.Error(".cursor/rules/always.mdc must stay when sync does not write AGENTS.md")
	}
}

// With "Include AGENTS.md in the context" switched on, the user tells
// sync so with rules-mode: entry-point and Trae reads AGENTS.md alone.
func TestSync_TraeSkipsAlwaysOnRulesWhenItDeclaresItReadsAGENTSMd(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, trae]\noutputs:\n  trae:\n    rules-mode: entry-point\n")

	mustSync(t)

	if exists(filepath.Join(dir, ".trae", "rules", "always.md")) {
		t.Error(".trae/rules/always.md loads the rule a second time next to AGENTS.md")
	}
	if !exists(filepath.Join(dir, ".trae", "rules", "go.md")) {
		t.Error(".trae/rules/go.md must stay: only the rules folder scopes it by path")
	}
}

// Codex owns the shared AGENTS.md. Adding the tools that read it must
// not change a byte of it.
func TestSync_CursorWindsurfAndTraeLeaveCodexAGENTSMdUnchanged(t *testing.T) {
	const opts = "outputs:\n  codex:\n    nested-glob-rules: false\n"
	dir := ruleFilesProject(t, "targets: [codex]\n"+opts)
	mustSync(t)
	alone := readFile(t, filepath.Join(dir, "AGENTS.md"))

	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, cursor, windsurf, trae]\n"+opts)
	mustSync(t)

	if got := readFile(t, filepath.Join(dir, "AGENTS.md")); got != alone {
		t.Errorf("AGENTS.md changed when Cursor, Windsurf and Trae joined codex:\nbefore:\n%s\nafter:\n%s", alone, got)
	}
}

func TestLintBudget_CursorAndWindsurfCountEachAlwaysOnRuleOnce(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cursor, windsurf]\nlint:\n  instructions-words: 1\n")

	out, _ := runCLI(t, "lint")

	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "always-on rule files") {
			t.Errorf("Cursor and Windsurf load the always-on rule from AGENTS.md only, got:\n%s", line)
		}
	}
}

// Trae reads AGENTS.md only once its switch is on, so by default lint
// counts its rule files and not AGENTS.md.
func TestLintBudget_TraeDoesNotCountAGENTSMdByDefault(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, trae]\nlint:\n  instructions-words: 1\n")

	out, _ := runCLI(t, "lint")

	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "trae") && strings.Contains(line, "AGENTS.md ") {
			t.Errorf("Trae does not read AGENTS.md by default, got:\n%s", line)
		}
	}
}

func TestExplainContext_CountsEachAlwaysOnRuleOnce(t *testing.T) {
	cases := map[string]string{
		"cursor":                  "targets: [codex, cursor]\n",
		"windsurf":                "targets: [codex, windsurf]\n",
		"trae":                    "targets: [codex, trae]\n",
		"trae with the switch on": "targets: [codex, trae]\noutputs:\n  trae:\n    rules-mode: entry-point\n",
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			ruleFilesProject(t, config)
			target, _, _ := strings.Cut(name, " ")

			out, err := runCLI(t, "explain", "--context", "--target", target)
			if err != nil {
				t.Fatalf("explain --context: %v\n%s", err, out)
			}
			var startup []string
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "[startup]") && strings.Contains(line, "rules/always.md") {
					startup = append(startup, line)
				}
			}
			if len(startup) != 1 {
				t.Errorf("%s loads the always-on rule once, got %d startup lines:\n%s", target, len(startup), out)
			}
		})
	}
}

func TestExplain_CreditsAGENTSMdToCursorForAnInlinedRule(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cursor]\n")

	out, err := runCLI(t, "explain", filepath.Join(".agnostic-ai", "rules", "always.md"))
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	if !strings.Contains(out, `[cursor] AGENTS.md (section "always")`) {
		t.Errorf("explain should credit the AGENTS.md section to cursor, got:\n%s", out)
	}
	if strings.Contains(out, ".cursor/rules/always.mdc") {
		t.Errorf("explain lists a .cursor/rules file sync no longer writes:\n%s", out)
	}
}

func TestRender_CursorShowsTheAGENTSMdSectionForAnInlinedRule(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cursor]\n")

	out, err := runCLI(t, "render", filepath.Join(".agnostic-ai", "rules", "always.md"), "-t", "cursor")
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}
	if !strings.Contains(out, "# target: cursor") || !strings.Contains(out, " AGENTS.md") || !strings.Contains(out, "Always body.") {
		t.Errorf("render should show the AGENTS.md section Cursor reads, got:\n%s", out)
	}
	if strings.Contains(out, ".cursor/rules/always.mdc") {
		t.Errorf("render shows a .cursor/rules file sync no longer writes:\n%s", out)
	}
}

func TestImport_CursorReadsTheRulesBlockItsCopiesWereSkippedFor(t *testing.T) {
	dir := ruleFilesProject(t, "targets: [codex, cursor]\n")
	mustSync(t)
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "rules")); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import cursor: %v\n%s", err, out)
	}

	if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md")), "Always body.") {
		t.Error("import cursor must recover the rule Cursor reads from AGENTS.md")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md")), "**/*.go") {
		t.Error("import cursor must keep the scoped rule from .cursor/rules/go.mdc")
	}
}

// upgradedProject leaves a .clinerules copy the next full sync removes,
// as an upgrade to a version that skips it does.
func upgradedProject(t *testing.T) string {
	t.Helper()
	dir := ruleFilesProject(t, "targets: [cline]\n")
	mustSync(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, cline]\n")
	mustSync(t, "--only", "codex")
	return dir
}

func TestDoctorFix_RemovesLeftovers(t *testing.T) {
	dir := upgradedProject(t)

	if out, err := runCLI(t, "doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix must settle a leftover: %v\n%s", err, out)
	}
	if exists(filepath.Join(dir, ".clinerules", "always.md")) {
		t.Error("doctor --fix left .clinerules/always.md in place")
	}
	if out, err := runCLI(t, "doctor"); err != nil {
		t.Errorf("doctor after --fix: %v\n%s", err, out)
	}
}

// The ledger does not record which target wrote a file, so leftovers
// get a report of their own instead of the entry-point one.
func TestSyncCheck_ReportsLeftoversUnderTheLedger(t *testing.T) {
	upgradedProject(t)

	out, _ := runCLI(t, "sync", "--check", "--json")

	if !strings.Contains(out, `"target": "ledger"`) {
		t.Errorf("leftover should report under target ledger:\n%s", out)
	}
}

// A rule that already has a spec is never imported again from the
// block, even when its file name differs from the rule name.
func TestImport_ClineSkipsBlockRulesThatAlreadyHaveASpec(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always-rule.md"), alwaysRuleSpec)
	mustSync(t)

	if out, err := runCLI(t, "import", "cline"); err != nil {
		t.Fatalf("import cline: %v\n%s", err, out)
	}

	if exists(filepath.Join(dir, ".agnostic-ai", "rules", "always.md")) {
		t.Error("import cline duplicated the always rule, which already has a spec")
	}
}

func TestRulesInEntryPoint_NamesTheAlwaysOnRulesAGENTSMdCarriesForCline(t *testing.T) {
	ruleFilesProject(t, "targets: [codex, cline]\n")
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}

	got := adapters.RulesInEntryPoint(cfg, b, "cline")

	if len(got) != 1 || !got["always"] {
		t.Errorf("RulesInEntryPoint(cline) = %v, want only always: the scoped go rule keeps its file", got)
	}
}
