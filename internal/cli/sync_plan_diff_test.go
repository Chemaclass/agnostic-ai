package cli

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_PlanDiffPrintsPlanThenDiffs(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)
	before := snapshotTree(t, dir)

	for _, args := range [][]string{
		{"sync", "-t", "claude", "--plan", "--diff"},
		{"sync", "-t", "claude", "--plan", "--check", "--diff"},
	} {
		var out bytes.Buffer
		root := NewRootCmd("test")
		root.SetOut(&out)
		root.SetArgs(args)
		err := root.Execute()
		check := strings.Contains(strings.Join(args, " "), "--check")
		if check && err == nil {
			t.Errorf("%v must exit non-zero on drift", args)
		}
		if !check && err != nil {
			t.Errorf("%v exits zero on drift, got %v", args, err)
		}
		got := out.String()
		plan := strings.Index(got, "[claude]")
		hunk := strings.Index(got, "--- .claude/rules/r1.md (on disk)")
		if plan < 0 || hunk < plan {
			t.Errorf("%v must print the plan, then the diff:\n%s", args, got)
		}
		for _, want := range []string{"changed: 1", "-hand-edited", "+rule body"} {
			if !strings.Contains(got, want) {
				t.Errorf("%v output missing %q:\n%s", args, want, got)
			}
		}
	}
	if after := snapshotTree(t, dir); !maps.Equal(before, after) {
		t.Error("--plan --diff must change no files")
	}
}

func TestSync_PlanDiffInSyncPrintsOnlyThePlan(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var plain, withDiff bytes.Buffer
	for _, run := range []struct {
		out  *bytes.Buffer
		args []string
	}{
		{&plain, []string{"sync", "-t", "claude", "--plan", "--check"}},
		{&withDiff, []string{"sync", "-t", "claude", "--plan", "--check", "--diff"}},
	} {
		root = NewRootCmd("test")
		root.SetOut(run.out)
		root.SetArgs(run.args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", run.args, err)
		}
	}
	if plain.String() != withDiff.String() {
		t.Errorf("in sync, --diff adds nothing to the plan:\nplan:\n%s\nwith --diff:\n%s", plain.String(), withDiff.String())
	}
}

func TestSync_PlanDiffRejectsJSON(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	for _, global := range []bool{false, true} {
		args := []string{"sync", "--plan", "--diff", "--json"}
		if global {
			globalAgentTestHome(t)
			args = append(args, "--global")
		}
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--json cannot be combined with") {
			t.Errorf("%v must be rejected, got %v", args, err)
		}
	}
}

func TestSyncGlobal_PlanDiffPrintsPlanThenDiffs(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	path := filepath.ToSlash(filepath.Join(home, ".claude", "CLAUDE.md"))
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one, revised.\n")
	before := snapshotTree(t, home)

	out, _, err := runGlobalAgentTest("--only", "claude", "--plan", "--diff")
	if err != nil {
		t.Fatalf("a plan exits zero on drift: %v", err)
	}
	plan := strings.Index(out, "update "+path)
	hunk := strings.Index(out, "--- "+path+" (on disk)")
	if plan < 0 || hunk < plan {
		t.Errorf("--plan --diff must print the plan, then the diff:\n%s", out)
	}
	for _, want := range []string{"-- Rule one.", "+- Rule one, revised."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--plan", "--check", "--diff"); err == nil {
		t.Error("--plan --check --diff must exit non-zero on drift")
	}
	if after := snapshotTree(t, home); !maps.Equal(before, after) {
		t.Error("--plan --diff must change no user files or state")
	}

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runGlobalAgentTest("--only", "claude", "--plan", "--check", "--diff")
	if err != nil || out != "No changes.\n" {
		t.Errorf("after sync the plan has no changes and no diff, got err = %v, out:\n%s", err, out)
	}
}

func TestSync_PlanDiffNamesARemoval(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "--plan", "--diff"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "would remove .claude/rules/r1.md") {
		t.Errorf("the plan diff must name the removal:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "rules", "r1.md")); err != nil {
		t.Errorf("previews must not remove anything: %v", err)
	}
}
