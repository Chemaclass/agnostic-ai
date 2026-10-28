package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// userJSONDotfiles are tracked hand-written files the ledger-less scan
// has to rule out: each sits where a target writes, has no provenance
// header, and was last changed by a commit of its own, so the scan renders
// the specs at that commit to look for proof.
const userJSONDotfiles = 9

func commitFixtureFiles(tb testing.TB, root, message string, paths ...string) {
	tb.Helper()
	stagedCheckGit(tb, root, append([]string{"add", "--"}, paths...)...)
	stagedCheckGit(tb, root, "-c", "user.name=benchmark", "-c", "user.email=benchmark@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath="+os.DevNull, "commit", "-q", "--no-verify", "-m", message)
}

// writeLedgerlessScanFixture builds a synced project of about files
// tracked files in thousands of directories, then commits userDotfiles
// hand-written JSON dotfiles one commit apiece. Deleting its
// `.agnostic-ai/.sync-state` gives a fresh clone's first sync.
func writeLedgerlessScanFixture(tb testing.TB, root string, specs, files, extraDirs, userDotfiles int) []string {
	tb.Helper()
	writeStagedCheckFixture(tb, root, specs, files-extraDirs-userDotfiles)
	var tracked []string
	for i := 0; i < extraDirs; i++ {
		p := filepath.Join("apps", fmt.Sprintf("app-%04d", i), "src", "index.ts")
		writeBenchFile(tb, filepath.Join(root, p), fmt.Sprintf("export const app%d = %d\n", i, i))
		tracked = append(tracked, p)
	}
	commitFixtureFiles(tb, root, "application sources", tracked...)
	var dotfiles []string
	for i := 0; i < userDotfiles; i++ {
		p := fmt.Sprintf(".tool-%d.json", i)
		writeBenchFile(tb, filepath.Join(root, p), fmt.Sprintf("{\"tool\": %d}\n", i))
		commitFixtureFiles(tb, root, fmt.Sprintf("configure tool %d", i), p)
		dotfiles = append(dotfiles, p)
	}
	// A clone holds packed objects; reading thousands of loose trees would
	// swamp the render this fixture measures.
	stagedCheckGit(tb, root, "repack", "-a", "-d", "-q")
	return dotfiles
}

// A fresh clone has no `.sync-state`, so its first sync sends every
// tracked file through the leftover scan. The renders of the committed
// specs are what that scan costs.
func BenchmarkSyncWithoutLedger(b *testing.B) {
	root := b.TempDir()
	writeLedgerlessScanFixture(b, root, 100, 23000, 3000, userJSONDotfiles)
	for _, tc := range []struct {
		name   string
		ledger bool
	}{{"fresh-clone", false}, {"ledger-present", true}} {
		b.Run(fmt.Sprintf("files=23000/%s", tc.name), func(b *testing.B) {
			state := filepath.Join(root, filepath.FromSlash(stateFilePath(".")))
			if err := runStagedCheckCommand(b, root, "sync"); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !tc.ledger {
					b.StopTimer()
					if err := os.Remove(state); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
				}
				if err := runStagedCheckCommand(b, root, "sync"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestUnledgeredScan_RendersEachCommitOnceAndCreatesNoUnrelatedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git invocation counter uses a shell wrapper")
	}
	root := t.TempDir()
	dotfiles := writeLedgerlessScanFixture(t, root, 4, 200, 80, 3)
	testutil.Chdir(t, root)
	if err := os.Remove(stateFilePath(".")); err != nil {
		t.Fatal(err)
	}
	log := recordRenderGitCalls(t)
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	emitted := map[string]bool{}
	rep := unledgeredReport(cfg, emitted, syncStateFile{}, ledgerMissing("."), strandedOutput(cfg, emitted, syncStateFile{}))

	for _, p := range dotfiles {
		for _, listed := range append(append([]string{}, rep.Leftover...), rep.Orphaned...) {
			if listed == p {
				t.Errorf("hand-written %s classified as generated: %+v", p, rep)
			}
		}
		if _, proven := rep.proven[p]; proven {
			t.Errorf("hand-written %s counted as proven: %+v", p, rep.proven)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	renders := 0
	for _, line := range lines {
		switch fields := strings.Fields(line); {
		case line == "read-tree":
			renders++
		case len(fields) == 2 && fields[0] == "dirs":
			var dirs int
			if _, err := fmt.Sscan(fields[1], &dirs); err != nil {
				t.Fatal(err)
			}
			if dirs > 12 {
				t.Errorf("a render created %d directories before its specs were checked out; the 80 application directories are no scope:\n%s", dirs, data)
			}
		}
	}
	if renders != len(dotfiles) {
		t.Errorf("rendered %d times for %d commits, want one render per commit:\n%s", renders, len(dotfiles), data)
	}
}

// recordRenderGitCalls wraps git to log each render's read-tree and the
// directories the render's scratch tree holds when its specs are checked
// out.
func recordRenderGitCalls(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	wrapper := `#!/bin/sh
case "$1" in
read-tree)
	case "$GIT_INDEX_FILE" in
	*agnostic-ai-history-config-*) ;;
	*) echo read-tree >> ` + adapters.ShellQuote(log) + ` ;;
	esac ;;
checkout-index)
	for arg in "$@"; do
		case "$arg" in
		--prefix=*) echo "dirs $(find "${arg#--prefix=}" -type d | wc -l | tr -d ' ')" >> ` + adapters.ShellQuote(log) + ` ;;
		esac
	done ;;
esac
exec ` + adapters.ShellQuote(gitPath) + ` "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}
