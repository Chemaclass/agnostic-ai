package cli

import (
	"os"
	"path/filepath"
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
	testutil.TempCwd(t)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, "agnostic-ai.yaml", ignoreRollbackConfig)

	ignoreRollbackFail(t, []string{"sync"})

	if _, err := os.Lstat(".gitignore"); !os.IsNotExist(err) {
		t.Errorf("failed sync left the .gitignore it created: %v", err)
	}
}

// sync --json keeps what targets wrote, so a failure after .gitignore
// leaves a block that still ignores it.
func TestSyncJSON_FailedWorktreeIncludeKeepsTheGitignoreItCreated(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, "agnostic-ai.yaml", ignoreRollbackConfig)

	ignoreRollbackFail(t, []string{"sync", "--json"})

	if !fileExists("AGENTS.md") {
		t.Fatal("sync --json did not keep AGENTS.md")
	}
	if !gitIgnored(t, dir, "AGENTS.md") {
		t.Errorf("AGENTS.md is exposed to git add after the failed sync")
	}
}

// The sweep sync --json undoes puts an orphan back beside the outputs it
// keeps, so the block must ignore both.
func TestSyncJSON_FailedWorktreeIncludeKeepsNewAndRestoredPathsIgnored(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, "agnostic-ai.yaml", ignoreRollbackConfig)
	mustWriteFile(t, ".agnostic-ai/skills/gone/SKILL.md", "---\nname: gone\ndescription: Gone.\n---\nOld.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if err := os.RemoveAll(".agnostic-ai/skills"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".agnostic-ai/agents/helper.md", "---\nname: helper\ndescription: Helps.\n---\nHelp.\n")

	ignoreRollbackFail(t, []string{"sync", "--json"})

	for _, path := range []string{".agents/skills/gone/SKILL.md", ".codex/agents/helper.toml"} {
		if !fileExists(path) {
			t.Errorf("%s is not on disk", path)
			continue
		}
		if !gitIgnored(t, dir, path) {
			t.Errorf("%s is exposed to git add:\n%s", path, readFile(t, ".gitignore"))
		}
	}
}

// The os error behind an ignore file failure already names the file, so
// the message names it once.
func TestIgnoreFileErrorsNameTheFileOnce(t *testing.T) {
	runSync := func(t *testing.T) error {
		_, err := runCLI(t, "sync")
		return err
	}
	cases := []struct {
		name, config, dir, file string
		run                     func(t *testing.T) error
	}{
		{"read .gitignore", ignoreRollbackConfig, ".gitignore", ".gitignore", runSync},
		{"read .worktreeinclude", ignoreRollbackConfig, worktreeIncludeFile, worktreeIncludeFile, runSync},
		{"write .gitignore", ignoreRollbackConfig + "  path: missing/.gitignore\n", "", filepath.Join("missing", ".gitignore"), runSync},
		{"packs add", ignoreRollbackConfig, ".gitignore", ".gitignore", func(*testing.T) error { return ensureManagedGitignore(".") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			captureLogOut(t)
			mustWriteFile(t, "agnostic-ai.yaml", c.config)
			if c.dir != "" {
				if err := os.Mkdir(c.dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			err := c.run(t)

			if err == nil {
				t.Fatalf("%s: got no error", c.name)
			}
			if n := strings.Count(err.Error(), c.file); n != 1 {
				t.Errorf("error names %s %d times, want once: %v", c.file, n, err)
			}
		})
	}
}
