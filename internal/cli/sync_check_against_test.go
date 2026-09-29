package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func againstConfig(commit string) string {
	return "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  commit: [" + commit + "]\n"
}

// committedProject syncs a claude project whose gitignore.commit lists
// commit, commits what Git tracks, and enters it.
func committedProject(t *testing.T, commit string) string {
	t.Helper()
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(againstConfig(commit)), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// checkAgainst runs `sync --check --against ref` and returns stdout, stderr
// and the error.
func checkAgainst(t *testing.T, ref string, extra ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	log := captureLogOut(t)
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"sync", "--check", "--against", ref}, extra...))
	err := root.Execute()
	return log.String() + stdout.String(), stderr.String(), err
}

func TestSyncCheckAgainstIndex_FailsWhenSpecIsStagedWithoutItsOutput(t *testing.T) {
	dir := committedProject(t, "instructions")
	if _, _, err := checkAgainst(t, "index"); err != nil {
		t.Fatalf("a synced index should pass: %v", err)
	}
	editRuleSpec(t, dir, "Staged rule.\n")
	git(t, dir, "add", ".agnostic-ai/rules/r1.md")
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}

	stdout, _, err := checkAgainst(t, "index")

	if err == nil {
		t.Fatal("the regenerated rule output is on disk but not staged, so the index check should fail")
	}
	if !strings.Contains(stdout, ".claude/rules/r1.md") {
		t.Errorf("the report should name the rule output, got:\n%s", stdout)
	}
	if _, _, err := runCheckPlain(t); err != nil {
		t.Errorf("the working tree is in sync, so plain --check should pass: %v", err)
	}

	git(t, dir, "add", "-A")
	if _, _, err := checkAgainst(t, "index"); err != nil {
		t.Errorf("staging the output should pass: %v", err)
	}
}

func TestSyncCheckAgainstHEAD_FailsOnStaleCommitEvenWhenWorkingTreeIsSynced(t *testing.T) {
	dir := committedProject(t, "instructions")
	editRuleSpec(t, dir, "Committed rule.\n")
	git(t, dir, "add", ".agnostic-ai/rules/r1.md")
	git(t, dir, "commit", "-q", "-m", "stale outputs")
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}

	stdout, _, err := checkAgainst(t, "HEAD")

	if err == nil {
		t.Fatal("HEAD holds a stale rule output, so the check should fail")
	}
	if !strings.Contains(stdout, ".claude/rules/r1.md") {
		t.Errorf("the report should name the rule output, got:\n%s", stdout)
	}
	if _, _, err := runCheckPlain(t); err != nil {
		t.Errorf("the working tree is in sync, so plain --check should pass: %v", err)
	}

	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "regenerate")
	if _, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("a commit holding the outputs should pass: %v", err)
	}
}

func TestSyncCheckAgainst_SkipsOutputsGitIgnores(t *testing.T) {
	dir := committedProject(t, "hooks")
	editRuleSpec(t, dir, "rule body\n\nAlso check rounding.\n")
	git(t, dir, "add", ".agnostic-ai/rules/r1.md")

	if stdout, _, err := checkAgainst(t, "index"); err != nil {
		t.Errorf("the rule output is ignored, so a stale copy is not drift: %v\n%s", err, stdout)
	}
}

func TestSyncCheckAgainst_ReportsCommittedOutputMissingFromRef(t *testing.T) {
	dir := committedProject(t, "instructions")
	git(t, dir, "rm", "-q", "--cached", "CLAUDE.md")

	stdout, _, err := checkAgainst(t, "index")

	if err == nil || !strings.Contains(stdout, "CLAUDE.md") {
		t.Errorf("an output missing from the index should fail and be named, got err=%v\n%s", err, stdout)
	}
}

func TestSyncCheckAgainst_ReadsSpecsFromTheRefNotTheWorkingTree(t *testing.T) {
	dir := committedProject(t, "instructions")
	editRuleSpec(t, dir, "Unstaged rule.\n")

	if stdout, _, err := checkAgainst(t, "index"); err != nil {
		t.Errorf("an unstaged spec edit is not in the index, so the index is still in sync: %v\n%s", err, stdout)
	}
}

func TestSyncCheckAgainst_RejectsBadUse(t *testing.T) {
	committedProject(t, "instructions")
	for name, args := range map[string][]string{
		"without --check": {"sync", "--against", "index"},
		"unknown ref":     {"sync", "--check", "--against", "main"},
		"with --global":   {"sync", "--check", "--global", "--against", "index"},
		"with --watch":    {"sync", "--check", "--watch", "--against", "index"},
	} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSyncCheckAgainst_NeedsAGitWorkTree(t *testing.T) {
	isolateGit(t)
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if _, _, err := checkAgainst(t, "index"); err == nil || !strings.Contains(err.Error(), "Git work tree") {
		t.Errorf("err = %v, want it to say --against needs a Git work tree", err)
	}
}

func runCheckPlain(t *testing.T) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"sync", "--check"})
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// A generated output that stays tracked after its spec is gone is a
// leftover in the state Git holds, whatever the local ledger says.
func TestSyncCheckAgainst_ReportsTrackedOutputNoSpecProduces(t *testing.T) {
	dir := committedProject(t, "instructions")
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/r2.md"), []byte("---\nname: r2\n---\nsecond rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "add r2")
	git(t, dir, "rm", "-q", ".agnostic-ai/rules/r2.md")

	for _, ref := range []string{"index", "HEAD"} {
		if ref == "HEAD" {
			git(t, dir, "commit", "-q", "-m", "drop the r2 spec")
		}
		stdout, _, err := checkAgainst(t, ref)
		if err == nil || !strings.Contains(stdout, ".claude/rules/r2.md") {
			t.Errorf("--against %s: a tracked output no spec produces should fail and be named, got err=%v\n%s", ref, err, stdout)
		}
	}

	git(t, dir, "rm", "-q", ".claude/rules/r2.md")
	git(t, dir, "commit", "-q", "-m", "drop the r2 output")
	if stdout, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("with the leftover removed, the check should pass: %v\n%s", err, stdout)
	}
}

// A pre-commit hook runs with GIT_DIR and GIT_INDEX_FILE pointing at the
// real repository; the leftover scan must still read the exported state.
func TestSyncCheckAgainst_ReportsLeftoverUnderHookEnvironment(t *testing.T) {
	dir := committedProject(t, "instructions")
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/r2.md"), []byte("---\nname: r2\n---\nsecond rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "add r2")
	git(t, dir, "rm", "-q", ".agnostic-ai/rules/r2.md")
	t.Setenv("GIT_DIR", filepath.Join(dir, ".git"))
	t.Setenv("GIT_INDEX_FILE", ".git/index")

	stdout, _, err := checkAgainst(t, "index")
	if err == nil || !strings.Contains(stdout, ".claude/rules/r2.md") {
		t.Errorf("under a hook environment, a staged spec deletion should fail on its tracked output, got err=%v\n%s", err, stdout)
	}
	if got := os.Getenv("GIT_INDEX_FILE"); got != ".git/index" {
		t.Errorf("GIT_INDEX_FILE after the check = %q, want it restored", got)
	}
}

// A repository with nothing staged yet has no index file.
func TestSyncCheckAgainstIndex_PassesWithNoIndexFile(t *testing.T) {
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	testutil.Chdir(t, dir)
	silence(t)
	if _, _, err := checkAgainst(t, "index"); err != nil && strings.Contains(err.Error(), "index") {
		t.Errorf("a missing index file should read as an empty index, got %v", err)
	}
}

// A deleted spec's output without a provenance header, such as Claude
// Code's launch.json, fails once the deletion is staged and once it is
// committed, since the previous state rendered it.
func TestSyncCheckAgainst_ReportsHeaderlessOutputOfADeletedSpec(t *testing.T) {
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\ngitignore:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\ndev-commands:\n  - name: Docs\n    command: npm run docs\n")
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "launch.json")); err != nil {
		t.Fatalf("sync wrote no launch.json: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	if _, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Fatalf("a synced commit should pass: %v", err)
	}

	git(t, dir, "rm", "-q", ".agnostic-ai/environments/dev.yaml")
	stdout, _, err := checkAgainst(t, "index")
	if err == nil || !strings.Contains(stdout, ".claude/launch.json") {
		t.Errorf("--against index: a staged spec deletion should fail on its launch.json, got err=%v\n%s", err, stdout)
	}
	git(t, dir, "commit", "-q", "-m", "drop the environment spec")
	stdout, _, err = checkAgainst(t, "HEAD")
	if err == nil || !strings.Contains(stdout, ".claude/launch.json") {
		t.Errorf("--against HEAD: a committed spec deletion should fail on its launch.json, got err=%v\n%s", err, stdout)
	}

	git(t, dir, "rm", "-q", ".claude/launch.json")
	git(t, dir, "commit", "-q", "-m", "drop launch.json")
	if stdout, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("with the output removed, the check should pass: %v\n%s", err, stdout)
	}
}

// Dropping a target from the config leaves its headerless output tracked,
// which fails; a dropped output edited since the last sync is left alone,
// since it may hold hand-written content.
func TestSyncCheckAgainst_ReportsOutputOfADroppedTargetUnlessEdited(t *testing.T) {
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	cfg := func(targets string) {
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: ["+targets+"]\ngitignore:\n  enabled: false\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg("claude, cursor")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\ninstall: npm ci\n")
	testutil.Chdir(t, dir)
	silence(t)
	sync := NewRootCmd("test")
	sync.SetArgs([]string{"sync"})
	if err := sync.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	envFile := filepath.Join(dir, ".cursor", "environment.json")
	if _, err := os.Stat(envFile); err != nil {
		t.Fatalf("sync wrote no .cursor/environment.json: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")

	cfg("claude")
	git(t, dir, "add", "agnostic-ai.yaml")
	stdout, _, err := checkAgainst(t, "index")
	if err == nil || !strings.Contains(stdout, ".cursor/environment.json") {
		t.Errorf("a dropped target's tracked output should fail, got err=%v\n%s", err, stdout)
	}
	if _, _, err := checkAgainst(t, "index", "--only", "claude"); err != nil && strings.Contains(err.Error(), "environment.json") {
		t.Errorf("a run narrowed to some targets should not compare dropped outputs: %v", err)
	}

	if err := os.WriteFile(envFile, []byte(`{"install": "npm ci", "note": "kept by hand"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".cursor/environment.json")
	if stdout, _, err := checkAgainst(t, "index"); err != nil && strings.Contains(stdout, "environment.json") {
		t.Errorf("a dropped output edited since sync should be left alone: %v\n%s", err, stdout)
	}
}
