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

	if !strings.HasPrefix(got, "  ~ rule testing, + skill deploy → 3 files in 2 targets\n") {
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

func TestGitPending_ReportsTrackedAndUntrackedButNotIgnored(t *testing.T) {
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

func TestGitPending_OutsideGitRepoReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if got := gitPending(dir, []string{"a.md"}); got != nil {
		t.Errorf("got %v", got)
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
