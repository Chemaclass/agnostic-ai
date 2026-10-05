package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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

// A branch from before a project moved its specs to .agnostic-ai/ can add
// a skill in a tool's generated folder. Git keeps tracking it inside the
// ignored folder, and only that tool reads it, so the check fails, names
// the file, and names the import that adopts it.
func TestSyncCheckAgainst_FailsOnHandWrittenConfigInAnIgnoredFolder(t *testing.T) {
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, cursor]\ngitignore:\n  enabled: true\n  commit: [cursor:reviews]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review a diff.\n---\nReview it.\n")
	testutil.Chdir(t, dir)
	silence(t)
	sync := NewRootCmd("test")
	sync.SetArgs([]string{"sync"})
	if err := sync.Execute(); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	if _, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Fatalf("a synced commit should pass: %v", err)
	}

	writeFile(t, filepath.Join(dir, ".cursor", "skills", "gh-stack", "SKILL.md"), "---\nname: gh-stack\ndescription: Stack PRs.\n---\nStack them.\n")
	git(t, dir, "add", "-f", ".cursor/skills/gh-stack/SKILL.md")
	git(t, dir, "commit", "-q", "-m", "a skill in the old place")

	stdout, stderr, err := checkAgainst(t, "HEAD", "--format", "github")
	if err == nil {
		t.Fatal("a hand-written skill tracked in an ignored folder should fail the check")
	}
	for _, want := range []string{"::error file=.cursor/skills/gh-stack/SKILL.md::", "agnostic-ai import cursor"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("annotation lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "agnostic-ai import cursor") {
		t.Errorf("hint lacks the import:\n%s", stderr)
	}
	if stdout, _, err := checkAgainst(t, "index"); err == nil {
		t.Errorf("the staged state holds the same file, so --against index should fail too:\n%s", stdout)
	}

	// Listing it under sync.unmanaged keeps it on purpose.
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, cursor]\ngitignore:\n  enabled: true\n  commit: [cursor:reviews]\nsync:\n  unmanaged: [.cursor/skills/gh-stack/]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "agnostic-ai.yaml")
	if stdout, _, err := checkAgainst(t, "index"); err != nil && strings.Contains(stdout, "gh-stack") {
		t.Errorf("a path under sync.unmanaged was reported:\n%s", stdout)
	}
}

// A hand-written file's fix is import, not regeneration, so the check
// does not tell the user to commit regenerated files.
func TestRegeneratedDrift_OnlyForOutputsSyncWrites(t *testing.T) {
	if regeneratedDrift([]driftReport{{Target: "unmanaged", Unmanaged: []unmanagedFinding{{Path: "a", Target: "cursor"}}}, {Target: unledgeredReportTarget, Orphaned: []string{"b"}}}) {
		t.Error("hand-written and leftover files asked for regeneration")
	}
	if !regeneratedDrift([]driftReport{{Target: "claude", Stale: []adapters.CapturedFile{{Path: "CLAUDE.md"}}}}) {
		t.Error("a stale output did not ask for regeneration")
	}
}

// The ledger describes the working tree, so a regenerated file that is
// synced but not staged was called a hand edit, with three conflicting
// next steps (#1592). It now names one: stage it.
func TestSyncCheckAgainstIndex_NamesStagingAsTheOneStep(t *testing.T) {
	dir := committedProject(t, "instructions")
	editRuleSpec(t, dir, "Staged rule.\n")
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", ".agnostic-ai/rules/r1.md")

	out, stderr, err := checkAgainst(t, "index")

	if err == nil || !strings.Contains(err.Error(), "run agnostic-ai sync, then stage the regenerated files with git add") {
		t.Fatalf("err = %v, want the staging step", err)
	}
	if !strings.Contains(out, "in the Git index do not match the specs there") {
		t.Errorf("drift not named as unstaged:\n%s", out)
	}
	for _, wrong := range []string{"edited locally", "to reconcile, run: agnostic-ai sync"} {
		if strings.Contains(out+stderr, wrong) {
			t.Errorf("output still says %q:\n%s%s", wrong, out, stderr)
		}
	}
}

func TestEnterAgainstTree_ExportsOnlyWhatTheCheckReads(t *testing.T) {
	dir := committedProject(t, "instructions")
	if err := os.MkdirAll(filepath.Join(dir, "src", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "app", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")

	tree, err := enterAgainstTree(againstIndex)
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	defer tree.leave()

	if fileExists(filepath.Join("src", "app", "main.go")) {
		t.Error("a tracked file the check never reads should stay out of the export")
	}
	for _, p := range []string{"CLAUDE.md", "agnostic-ai.yaml", ".gitignore", filepath.Join(".agnostic-ai", "rules", "r1.md")} {
		if !fileExists(p) {
			t.Errorf("%s should be exported", p)
		}
	}
}

func TestAgainstTreeSameInputs_OnlyWhenSpecsAndDirectoriesMatch(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, dir string)
		want  bool
	}{
		{"unchanged", func(t *testing.T, dir string) {}, true},
		{"a source file changed", func(t *testing.T, dir string) {
			mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\n")
		}, true},
		{"a spec changed", func(t *testing.T, dir string) { editRuleSpec(t, dir, "Changed rule.\n") }, false},
		{"a source directory was added", func(t *testing.T, dir string) {
			mustWriteFile(t, filepath.Join(dir, "src", "app", "main.go"), "package main\n")
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := committedProject(t, "instructions")
			mustWriteFile(t, filepath.Join(dir, "main.go"), "package base\n")
			git(t, dir, "add", "-A")
			git(t, dir, "commit", "-q", "-m", "source")
			tc.stage(t, dir)
			git(t, dir, "add", "-A")
			tree, err := enterAgainstTree(againstIndex)
			if err != nil {
				t.Fatalf("enter: %v", err)
			}
			defer tree.leave()

			got, err := tree.sameInputs("HEAD")

			if err != nil || got != tc.want {
				t.Errorf("sameInputs = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestAgainstTreeExportMissing_ChecksOutATrackedOutputTheGuessLeftOut(t *testing.T) {
	dir := committedProject(t, "instructions")
	mustWriteFile(t, filepath.Join(dir, "docs", "guide.md"), "tracked\n")
	git(t, dir, "add", "-A")
	tree, err := enterAgainstTree(againstIndex)
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	defer tree.leave()
	guide := filepath.Join("docs", "guide.md")
	untracked := filepath.Join("docs", "new.md")
	reports := []driftReport{{Target: "claude", Missing: []adapters.CapturedFile{{Path: guide}, {Path: untracked}}}}

	late, err := tree.exportMissing(reports)

	if err != nil || !late {
		t.Fatalf("exportMissing = %v, %v, want true", late, err)
	}
	if !fileExists(guide) {
		t.Error("the tracked output should now be in the export")
	}
	if fileExists(untracked) {
		t.Error("a path Git does not track has nothing to check out")
	}
	if late, _ := tree.exportMissing(reports); late {
		t.Error("an output already exported should not ask for a second pass")
	}
}

func TestSyncCheckAgainst_PassesWithAnIncludeGitDoesNotTrack(t *testing.T) {
	dir := committedProject(t, "instructions")
	entry := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, entry, string(data)+"\n@~/.claude/personal.md\n")
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "include")

	if _, stderr, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("a reference outside the repository is not an input to export: %v\n%s", err, stderr)
	}
}

func TestSyncCheckAgainst_ReadsASpecLinkedOutsideTheDotPaths(t *testing.T) {
	dir := committedProject(t, "instructions")
	entry := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "shared", "instructions.md"), string(data)+"\nShared line.\n")
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "shared", "instructions.md"), entry); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "linked instructions")

	if stdout, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("the linked spec should be exported with its target: %v\n%s", err, stdout)
	}
}

func TestSyncCheckAgainst_ReadsALinkedSourceDirectory(t *testing.T) {
	dir := committedProject(t, "instructions")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), againstConfig("instructions")+"sources:\n  rules: rules\n")
	mustWriteFile(t, filepath.Join(dir, "policies", "r9.md"), "---\nalwaysApply: true\n---\nLinked rule.\n")
	if err := os.Symlink("policies", filepath.Join(dir, "rules")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "linked rules")

	if stdout, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("the linked source directory should be exported with its target: %v\n%s", err, stdout)
	}
}

func TestSyncCheckAgainst_ComparesAScopedOutputWithoutItsDirectoryExported(t *testing.T) {
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\ngitignore:\n  enabled: true\n  commit: [instructions]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"), "---\nname: api\nglobs: [src/api/**]\n---\nAPI rule.\n")
	mustWriteFile(t, filepath.Join(dir, "src", "api", "handler.go"), "package api\n")
	testutil.Chdir(t, dir)
	silence(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !fileExists(filepath.Join(dir, "src", "api", "AGENTS.md")) {
		t.Fatal("sync should write the scoped AGENTS.md")
	}
	git(t, dir, "add", "-A")
	git(t, dir, "add", "-f", "src/api/AGENTS.md")
	git(t, dir, "commit", "-q", "-m", "base")
	if stdout, _, err := checkAgainst(t, "HEAD"); err != nil {
		t.Fatalf("a synced commit should pass: %v\n%s", err, stdout)
	}

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"), "---\nname: api\nglobs: [src/api/**]\n---\nChanged API rule.\n")
	git(t, dir, "add", ".agnostic-ai/rules/api.md")

	stdout, _, err := checkAgainst(t, "index")

	if err == nil || !strings.Contains(stdout, "src/api/AGENTS.md") {
		t.Errorf("the staged rule should report its scoped output, got %v:\n%s", err, stdout)
	}
}
