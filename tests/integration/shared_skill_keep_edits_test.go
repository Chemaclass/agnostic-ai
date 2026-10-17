package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Codex and Amp both write .agents/skills/. A spec change synced with
// --keep-edits must update every shared file, never keep one as a hand
// edit because the other target was mid-write (#2245).
func TestSyncKeepEdits_SharedSkillFilesFollowSpecChanges(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, amp]\n")
	const rules = 60
	writeSpecs := func(version int) {
		mustWrite(t, filepath.Join(dir, ".agnostic-ai/skills/shared/SKILL.md"),
			"---\nname: shared\ndescription: Shared skill fixture.\n---\nUse the rules.\n")
		for i := range rules {
			mustWrite(t, filepath.Join(dir, fmt.Sprintf(".agnostic-ai/skills/shared/rules/r%02d.md", i)),
				strings.Repeat(fmt.Sprintf("Rule %d, version %d.\n", i, version), 4000))
		}
	}
	run := func(args ...string) {
		t.Helper()
		root := cli.NewRootCmd("test")
		root.SetArgs(args)
		root.SetOut(&strings.Builder{})
		root.SetErr(&strings.Builder{})
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	writeSpecs(0)
	run("sync")
	for version := 1; version <= 6; version++ {
		writeSpecs(version)
		run("sync", "--keep-edits", "--jobs", "8")
		for i := range rules {
			rel := fmt.Sprintf(".agents/skills/shared/rules/r%02d.md", i)
			got, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("Rule %d, version %d.\n", i, version); !strings.HasPrefix(string(got), want) {
				t.Fatalf("version %d: %s kept stale content %q", version, rel, string(got)[:min(len(got), 40)])
			}
		}
		run("sync", "--check")
	}
}
