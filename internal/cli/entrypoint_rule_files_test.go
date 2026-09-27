package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
