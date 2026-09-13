package integration

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setupSkillReferencesProject writes a single-target project with a kept
// skill and a skill `gone` that bundles reference files, then moves into it.
func setupSkillReferencesProject(t *testing.T, target string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
	mustWrite(t, ".agnostic-ai/AGNOSTIC_AI.md", "# A\n")
	mustWrite(t, ".agnostic-ai/skills/keep/SKILL.md", "---\nname: keep\ndescription: keep\n---\nbody\n")
	mustWrite(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: gone\n---\nbody\n")
	for _, f := range []string{"a", "b", "c"} {
		mustWrite(t, ".agnostic-ai/skills/gone/references/"+f+".md", "ref\n")
	}
}

// goneOutputs lists every emitted path outside the source tree that
// belongs to the `gone` skill.
func goneOutputs(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (p == ".agnostic-ai" || p == ".git") {
			return filepath.SkipDir
		}
		if strings.Contains(filepath.ToSlash(p), "/gone") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(out)
	return out
}

type syncStateForTest struct {
	Outputs []string `json:"outputs"`
	Orphans []string `json:"orphans"`
}

func readSyncState(t *testing.T) syncStateForTest {
	t.Helper()
	var s syncStateForTest
	must(t, json.Unmarshal([]byte(readString(t, ".agnostic-ai/.sync-state")), &s))
	return s
}

// Deleting a skill that bundles reference files removes its whole emitted
// folder in every target, and the ledger forgets nothing that is still on
// disk (#785).
func TestSync_DeletedSkillRemovesBundledReferencesInEveryTarget(t *testing.T) {
	targets := adapters.Names()
	sort.Strings(targets)
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			setupSkillReferencesProject(t, target)
			runCmd(t, "sync")

			must(t, os.RemoveAll(".agnostic-ai/skills/gone"))
			runCmd(t, "sync")

			if left := goneOutputs(t); len(left) > 0 {
				t.Errorf("deleted skill left files behind: %v", left)
			}
			for _, p := range readSyncState(t).Outputs {
				if strings.Contains(p, "/gone/") {
					t.Errorf("ledger still lists %s", p)
				}
			}
			runCmd(t, "sync", "--check")
		})
	}
}

// A reference edited by hand after sync is kept, reported, and fails the
// check until the user deletes it.
func TestSync_EditedReferenceOfDeletedSkillIsKeptAndFailsCheck(t *testing.T) {
	setupSkillReferencesProject(t, "claude")
	runCmd(t, "sync")
	const edited = ".claude/skills/gone/references/a.md"
	mustWrite(t, edited, "my notes\n")

	must(t, os.RemoveAll(".agnostic-ai/skills/gone"))
	runCmd(t, "sync")

	if got := readString(t, edited); got != "my notes\n" {
		t.Errorf("edited reference changed: %q", got)
	}
	want := []string{".claude/skills/gone", ".claude/skills/gone/references", edited}
	if left := goneOutputs(t); strings.Join(left, ",") != strings.Join(want, ",") {
		t.Errorf("left behind %v, want only the edited reference and its folders %v", left, want)
	}
	state := readSyncState(t)
	if len(state.Orphans) != 1 || state.Orphans[0] != edited {
		t.Errorf("orphans=%v, want [%s]", state.Orphans, edited)
	}
	runCmdExpectErr(t, "sync", "--check")

	must(t, os.Remove(edited))
	runCmd(t, "sync")
	runCmd(t, "sync", "--check")
	if state := readSyncState(t); len(state.Orphans) != 0 {
		t.Errorf("orphans=%v after manual delete, want none", state.Orphans)
	}
}
