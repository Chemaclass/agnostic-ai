package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// chainProject writes a monorepo whose codex chain for apps/engine/src is
// about 28 KB before any review lands: 12 KB of root instructions, a 4 KB
// apps/engine rule, and a 12 KB apps/engine/src rule.
func chainProject(t *testing.T, config string) string {
	t.Helper()
	dir := budgetProject(t, "targets: [codex, cursor]\n"+config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), strings.Repeat("rootwords12 ", 1000)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "engine.md"), "---\nname: engine\nscope: apps/engine\n---\n"+strings.Repeat("eng ", 1000)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "src.md"), "---\nname: src\nscope: apps/engine/src\n---\n"+strings.Repeat("src ", 3000)+"\n")
	return dir
}

func chainFindings(out string) []string {
	var lines []string
	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "Codex reads") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestLintCodexChain_WarnsWhenScopeChainPassesTheDefault(t *testing.T) {
	dir := chainProject(t, "")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "src.md"), "---\nscope: apps/engine/src\n---\n"+strings.Repeat("rev ", 1500)+"\n")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("a chain warning alone must not fail lint: %v\n%s", err, out)
	}
	lines := chainFindings(out)
	if len(lines) != 1 {
		t.Fatalf("expected one chain warning, got:\n%s", out)
	}
	for _, want := range []string{
		"[warn] apps/engine/src/AGENTS.md",
		"in scope apps/engine/src,",
		"past 32768",
		"AGENTS.md 12",
		"apps/engine/AGENTS.md 4",
		"apps/engine/src/AGENTS.md 1",
		"reviews 6",
		"project_doc_max_bytes",
	} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("chain warning lacks %q:\n%s", want, lines[0])
		}
	}
	if _, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Error("lint --strict must fail on a chain warning")
	}
}

func TestLintCodexChain_QuietOnceTheReviewMovesToCursor(t *testing.T) {
	dir := chainProject(t, "")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "reviews", "src.md"), "---\nscope: apps/engine/src\ntarget: cursor\n---\n"+strings.Repeat("rev ", 1500)+"\n")

	out, err := runCLI(t, "lint", "--strict")
	if err != nil {
		t.Fatalf("the chain is under 32 KiB without the review: %v\n%s", err, out)
	}
	if lines := chainFindings(out); len(lines) != 0 {
		t.Errorf("unexpected chain warning:\n%s", out)
	}
}

func TestLintCodexChain_HonorsTheConfiguredLimit(t *testing.T) {
	dir := chainProject(t, "lint:\n  codex-chain-bytes: 20000\n")

	out, _ := runCLI(t, "lint")
	lines := chainFindings(out)
	if len(lines) != 1 || !strings.Contains(lines[0], "past 20000") || !strings.Contains(lines[0], "lint.codex-chain-bytes") {
		t.Errorf("expected one chain warning against the 20000-byte limit, got:\n%s", out)
	}

	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, cursor]\nlint:\n  codex-chain-bytes: 60000\n")
	out, _ = runCLI(t, "lint")
	if lines := chainFindings(out); len(lines) != 0 {
		t.Errorf("a 60000-byte limit leaves this chain quiet, got:\n%s", out)
	}
}

func TestLintCodexChain_ReportsOnlyTheFirstScopePastTheLimit(t *testing.T) {
	dir := chainProject(t, "lint:\n  codex-chain-bytes: 15000\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "deep.md"), "---\nname: deep\nscope: apps/engine/src/deep\n---\n"+strings.Repeat("deep ", 200)+"\n")

	out, _ := runCLI(t, "lint")
	lines := chainFindings(out)
	if len(lines) != 1 || !strings.Contains(lines[0], "in scope apps/engine,") {
		t.Errorf("expected one warning at apps/engine, the first scope past the limit, got:\n%s", out)
	}
}

func TestLintCodexChain_IgnoresProjectsWithoutCodex(t *testing.T) {
	dir := budgetProject(t, "targets: [cursor, amp]\nlint:\n  codex-chain-bytes: 100\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), strings.Repeat("root ", 400)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "engine.md"), "---\nname: engine\nscope: apps/engine\n---\n"+strings.Repeat("eng ", 400)+"\n")

	out, _ := runCLI(t, "lint")
	if lines := chainFindings(out); len(lines) != 0 {
		t.Errorf("Codex is not a target, so no chain warning is expected:\n%s", out)
	}
}

func TestLintCodexChain_RejectsNegativeLimit(t *testing.T) {
	budgetProject(t, "targets: [codex]\nlint:\n  codex-chain-bytes: -1\n")
	out, err := runCLI(t, "lint")
	if err == nil || !strings.Contains(err.Error()+out, "lint.codex-chain-bytes") {
		t.Errorf("expected a config error naming lint.codex-chain-bytes, got %v\n%s", err, out)
	}
}
