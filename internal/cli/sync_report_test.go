package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func renderReport(r syncReport, targets int, verbose bool) string {
	var buf bytes.Buffer
	r.render(&buf, targets, 5*time.Millisecond, verbose)
	return buf.String()
}

func TestSyncReport_NoChangesSaysUpToDate(t *testing.T) {
	got := renderReport(syncReport{}, 3, false)

	if got != "✓ 3 targets up to date · 5ms\n" {
		t.Errorf("got %q", got)
	}
}

func TestSyncReport_SummaryCountsEachActionAndSkipsZeros(t *testing.T) {
	var r syncReport
	r.addWrites("claude", []adapters.WrittenFile{
		{Path: "CLAUDE.md", Action: "update"},
		{Path: ".claude/rules/a.md", Action: "create"},
		{Path: ".claude/rules/b.md", Action: "skip"},
	})

	got := renderReport(r, 1, false)

	want := "  + .claude/rules/a.md\n  ~ CLAUDE.md\n✓ synced 1 target · 1 created · 1 updated · 5ms\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSyncReport_CapsPathsUnlessVerbose(t *testing.T) {
	r := syncReport{removed: []string{"a", "b", "c", "d", "e"}}

	if got := renderReport(r, 1, false); !strings.Contains(got, "  - a  b  c  (+2 more)\n") {
		t.Errorf("default output not capped:\n%s", got)
	}
	if got := renderReport(r, 1, true); !strings.Contains(got, "  - e\n") || strings.Contains(got, "more") {
		t.Errorf("verbose output should list every path:\n%s", got)
	}
}

func TestSyncReport_SpecLineNamesSourcesAndFanOut(t *testing.T) {
	var r syncReport
	r.specs = []specChange{{mark: "~", label: "rule testing"}, {mark: "+", label: "skill deploy"}}
	r.addWrites("claude", []adapters.WrittenFile{{Path: "a", Action: "update"}, {Path: "b", Action: "create"}})
	r.addWrites("cursor", []adapters.WrittenFile{{Path: "c", Action: "update"}})

	got := renderReport(r, 2, false)

	if !strings.HasPrefix(got, "  ~ rule testing  + skill deploy → 3 files in 2 targets\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestSyncReport_FanOutOmitsTargetsWhenFilesWereRemoved(t *testing.T) {
	r := syncReport{specs: []specChange{{mark: "-", label: "rule old"}}, removed: []string{"x"}}
	r.addWrites("claude", []adapters.WrittenFile{{Path: "CLAUDE.md", Action: "update"}})

	if got := renderReport(r, 1, false); !strings.HasPrefix(got, "  - rule old → 2 files\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestSyncReport_PendingLineListsFilesToCommit(t *testing.T) {
	r := syncReport{updated: []string{"AGENTS.md"}, pending: []string{"AGENTS.md"}}

	if got := renderReport(r, 1, false); !strings.Contains(got, "  ! 1 file to commit: AGENTS.md\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestDiffSpecSums(t *testing.T) {
	if got := diffSpecSums(nil, map[string]string{"rule a": "1"}); got != nil {
		t.Errorf("no baseline should report nothing, got %v", got)
	}

	got := diffSpecSums(
		map[string]string{"rule a": "1", "rule b": "1", "rule c": "1"},
		map[string]string{"rule a": "1", "rule b": "2", "rule d": "1"},
	)

	want := []specChange{{"~", "rule b"}, {"-", "rule c"}, {"+", "rule d"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEntrySum_SkillAssetEditChangesSum(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "SKILL.md")
	mustWriteFile(t, skill, "body")
	asset := filepath.Join(dir, "references", "notes.md")
	mustWriteFile(t, asset, "v1")
	e := spec.Entry{Kind: spec.KindSkill, Name: "s", Path: skill, Body: "body"}
	before := entrySum(e)

	mustWriteFile(t, asset, "v2")

	if entrySum(e) == before {
		t.Error("editing a skill asset should change the skill's sum")
	}
}

// gitRepo initializes a git repository in a fresh temp dir and returns the
// dir plus a helper that runs git there.
func gitRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	return dir, git
}

func TestGitPending_ReportsTrackedAndUntrackedButNotIgnored(t *testing.T) {
	dir, git := gitRepo(t)
	mustWriteFile(t, filepath.Join(dir, ".gitignore"), "ignored.md\n")
	mustWriteFile(t, filepath.Join(dir, "tracked.md"), "v1")
	mustWriteFile(t, filepath.Join(dir, "same.md"), "v1")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	mustWriteFile(t, filepath.Join(dir, "tracked.md"), "v2")
	mustWriteFile(t, filepath.Join(dir, "ignored.md"), "x")
	mustWriteFile(t, filepath.Join(dir, "new.md"), "x")

	got := gitPending(dir, []string{"tracked.md", "ignored.md", "new.md", "same.md"})

	if want := []string{"new.md", "tracked.md"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// `git rm --cached` on an output the gitignore block now covers stages
// its deletion while sync rewrites the file. That deletion is the user's
// own step, so the hint does not list the file as one to commit; a staged
// deletion of a file that is gone stays listed.
func TestGitPending_SkipsAnOutputBeingUntracked(t *testing.T) {
	dir, git := gitRepo(t)
	sub := filepath.Join(dir, "sub")
	mustWriteFile(t, filepath.Join(sub, "settings.json"), "v1")
	mustWriteFile(t, filepath.Join(sub, "gone.md"), "v1")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	git("rm", "-q", "--cached", "sub/settings.json")
	git("rm", "-q", "sub/gone.md")
	mustWriteFile(t, filepath.Join(sub, ".gitignore"), "settings.json\n")
	mustWriteFile(t, filepath.Join(sub, "settings.json"), "v2")

	got := gitPending(sub, []string{"settings.json", "gone.md"})

	if want := []string{"gone.md"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGitPending_PathsAreRelativeToAProjectInASubdirectory(t *testing.T) {
	dir, _ := gitRepo(t)
	sub := filepath.Join(dir, "sub")
	mustWriteFile(t, filepath.Join(sub, "new.md"), "x")

	got := gitPending(sub, []string{"new.md"})

	if want := []string{"new.md"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEntrySum_KeyOrderChangesSum(t *testing.T) {
	a := spec.Entry{Kind: spec.KindRule, Name: "r", MetaKeys: []string{"description", "globs"}}
	b := a
	b.MetaKeys = []string{"globs", "description"}

	if entrySum(a) == entrySum(b) {
		t.Error("reordering frontmatter keys should change the sum")
	}
}

func TestGitPending_OutsideGitRepoReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if got := gitPending(dir, []string{"a.md"}); got != nil {
		t.Errorf("got %v", got)
	}
}

// Without --literal-pathspecs, a path holding a glob character is read
// as a pattern: "a[X]b.txt" would also match and remove "aXb.txt" (#1330
// review).
func TestGitRmCached_TreatsGlobCharactersLiterally(t *testing.T) {
	dir, gitc := gitRepo(t)
	mustWriteFile(t, filepath.Join(dir, "a[X]b.txt"), "v1")
	mustWriteFile(t, filepath.Join(dir, "aXb.txt"), "v2")
	gitc("add", "-A")
	gitc("commit", "-q", "-m", "base")

	removed, err := gitRmCached(dir, []string{"a[X]b.txt"})

	if err != nil {
		t.Fatalf("gitRmCached: %v", err)
	}
	if want := []string{"a[X]b.txt"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	tracked := git(t, dir, "ls-files")
	if !strings.Contains(tracked, "aXb.txt") {
		t.Errorf("aXb.txt must stay tracked, got:\n%s", tracked)
	}
	if strings.Contains(tracked, "a[X]b.txt") {
		t.Errorf("a[X]b.txt should have been untracked, got:\n%s", tracked)
	}
}

// git rm --cached validates every pathspec in one invocation before
// touching the index: with a mix of a removable and a nonexistent
// path, a naive single call removes nothing. gitRmCached must still
// remove what it can and name only the path that failed (#1330 review).
func TestGitRmCached_PartialFailureRemovesWhatItCanAndNamesTheRest(t *testing.T) {
	dir, gitc := gitRepo(t)
	mustWriteFile(t, filepath.Join(dir, "good.txt"), "v1")
	gitc("add", "-A")
	gitc("commit", "-q", "-m", "base")

	removed, err := gitRmCached(dir, []string{"good.txt", "missing.txt"})

	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("err = %v, want it to name missing.txt", err)
	}
	if strings.Contains(err.Error(), "good.txt") {
		t.Errorf("err = %v, must not blame good.txt too", err)
	}
	if want := []string{"good.txt"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	tracked := git(t, dir, "ls-files")
	if strings.Contains(tracked, "good.txt") {
		t.Errorf("good.txt should have been untracked despite the other path failing, got:\n%s", tracked)
	}
}

func TestRunSyncOnce_ReportNamesTheEditedSpec(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "testing.md")
	mustWriteFile(t, rule, "---\ndescription: Tests.\n---\n\nWrite tests.\n")
	silence(t)
	buf := captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rule, []byte("---\ndescription: Tests.\n---\n\nWrite more tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "  ~ rule testing → ") {
		t.Errorf("missing spec line in:\n%s", out)
	}
	if !strings.Contains(out, "✓ synced 2 targets · ") || !strings.Contains(out, " updated · ") {
		t.Errorf("missing summary counts in:\n%s", out)
	}
}

func TestRunSyncOnce_PartialRunKeepsTheSpecBaseline(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
	rule := filepath.Join(dir, ".agnostic-ai", "rules", "testing.md")
	mustWriteFile(t, rule, "---\ndescription: Tests.\n---\n\nWrite tests.\n")
	silence(t)
	buf := captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, rule, "---\ndescription: Tests.\n---\n\nWrite more tests.\n")
	if err := runSyncOnce(".", []string{"claude"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	buf.Reset()

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "  ~ rule testing → ") {
		t.Errorf("full run after a partial one should still name the spec:\n%s", buf.String())
	}
}

func TestRunSyncOnce_ReportsAGitignoreRewrite(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "testing.md"), "---\ndescription: Tests.\n---\n\nWrite tests.\n")
	silence(t)
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "on", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), ".gitignore") {
		t.Errorf("first run with gitignore on should list .gitignore:\n%s", buf.String())
	}
	buf.Reset()

	if err := runSyncOnce(".", nil, false, false, "on", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), ".gitignore") {
		t.Errorf("an unchanged .gitignore should not be listed:\n%s", buf.String())
	}
}
