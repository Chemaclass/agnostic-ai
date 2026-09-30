package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const ignoreRollbackConfig = "version: 1\ntargets: [codex]\ngitignore:\n  enabled: true\n"

// ignoreRollbackFail breaks .worktreeinclude, the last ignore file a sync
// writes, and runs a sync that must fail after writing .gitignore.
func ignoreRollbackFail(t *testing.T, args []string) {
	t.Helper()
	// A directory fails the read on every OS, even for root.
	if err := os.Mkdir(worktreeIncludeFile, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, args...); err == nil {
		t.Fatalf("%s succeeded with %s a directory:\n%s", strings.Join(args, " "), worktreeIncludeFile, out)
	}
}

// A sync that fails after its sweep puts the orphan back, and .gitignore
// with it, so the block still ignores the orphan the ledger still names.
func TestSync_FailedIgnoreWriteRestoresGitignoreOfSweptOrphan(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			captureLogOut(t)
			mustWriteFile(t, "agnostic-ai.yaml", ignoreRollbackConfig)
			mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Gone.\n---\nOld.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			const orphan = ".agents/skills/gone/SKILL.md"
			before := readFile(t, ".gitignore")
			if !fileExists(orphan) || !strings.Contains(before, "/.agents/skills/\n") {
				t.Fatalf("first sync did not write and ignore %s:\n%s", orphan, before)
			}
			if err := os.RemoveAll(".agnostic-ai/skills"); err != nil {
				t.Fatal(err)
			}

			ignoreRollbackFail(t, args)

			if !fileExists(orphan) {
				t.Fatalf("swept orphan %s not restored", orphan)
			}
			if got := readFile(t, ".gitignore"); got != before {
				t.Errorf("failed sync left its .gitignore:\n%s",
					labeledDiff("before", "after", splitLines(before), splitLines(got), diffBodyMax))
			}
		})
	}
}

func TestSync_FailedIgnoreWriteRemovesTheGitignoreItCreated(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			captureLogOut(t)
			mustWriteFile(t, "agnostic-ai.yaml", ignoreRollbackConfig)

			ignoreRollbackFail(t, args)

			if _, err := os.Lstat(".gitignore"); !os.IsNotExist(err) {
				t.Errorf("failed sync left the .gitignore it created: %v", err)
			}
		})
	}
}
