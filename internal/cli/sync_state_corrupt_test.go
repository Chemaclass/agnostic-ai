package cli

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// truncateLedger syncs the fixture, then cuts the ledger in half, the
// state a full disk or a kill mid-write leaves behind.
func truncateLedger(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, setupFixture(t))
	silence(t)
	if out, err := runCLI(t, "sync", "-t", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data := readFile(t, stateFilePath("."))
	mustWriteFile(t, stateFilePath("."), data[:len(data)/2])
}

func TestLedgerMissing_TrueForALedgerThatDoesNotParse(t *testing.T) {
	truncateLedger(t)

	if !ledgerMissing(".") {
		t.Error("a truncated ledger counts as a ledger, so the leftover scan never runs")
	}
}

func TestSync_WarnsAboutATruncatedLedgerAndRebuildsIt(t *testing.T) {
	truncateLedger(t)

	var err error
	stderr := captureStderr(t, func() {
		_, err = runCLI(t, "sync", "-t", "claude")
	})

	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(stderr, ".agnostic-ai/.sync-state") || !strings.Contains(stderr, "does not parse") {
		t.Errorf("sync did not warn about the truncated ledger, stderr:\n%s", stderr)
	}
	state, err := readStateFileStrict(".")
	if err != nil || len(state.Outputs) == 0 {
		t.Errorf("sync did not rebuild the ledger: %v, %+v", err, state)
	}
}

func TestSyncCheck_ReportsATruncatedLedger(t *testing.T) {
	truncateLedger(t)

	out, err := runCLI(t, "sync", "-t", "claude", "--check")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/.sync-state") {
		t.Errorf("check passed on a truncated ledger: %v\n%s", err, out)
	}
}

// Each writer replaces the ledger with a new file instead of writing
// into the old one, so a stop mid-write leaves the old ledger whole.
func TestStateFileWriters_ReplaceTheLedgerWhole(t *testing.T) {
	writers := map[string]func() error{
		"writeStateFile": func() error {
			return writeStateFile(".", 1, "", "", syncLedger{outputs: []string{"CLAUDE.md"}})
		},
		"recordOutputSums": func() error {
			return recordOutputSums(".", map[string]string{"CLAUDE.md": "sum"})
		},
		"updateStateFile": func() error {
			return updateStateFile(".", func(s *syncStateFile) { s.FilesChanged = 2 })
		},
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, stateFilePath("."), "{}")
			// On Windows os.Stat leaves the file id for os.SameFile to read
			// later, from whatever file then holds the path; File.Stat reads
			// it now. Windows also refuses to replace an open file.
			f, err := os.Open(stateFilePath("."))
			if err != nil {
				t.Fatal(err)
			}
			before, err := f.Stat()
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				t.Fatal(err)
			}

			if err := write(); err != nil {
				t.Fatal(err)
			}

			after, err := os.Stat(stateFilePath("."))
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(before, after) {
				t.Error("the ledger was written in place")
			}
			if runtime.GOOS != "windows" && after.Mode().Perm() != 0o644 {
				t.Errorf("mode = %v, want 0644", after.Mode().Perm())
			}
		})
	}
}

func TestSyncPlanCheck_ReportsATruncatedLedger(t *testing.T) {
	truncateLedger(t)

	out, err := runCLI(t, "sync", "-t", "claude", "--plan", "--check")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/.sync-state") {
		t.Errorf("plan check passed on a truncated ledger: %v\n%s", err, out)
	}
}

// A truncated ledger was written by an earlier sync, so it is no reason
// to ask for targets as on a first sync.
func TestShouldPromptTargetSelection_FalseForATruncatedLedger(t *testing.T) {
	truncateLedger(t)

	if shouldPromptTargetSelection(".", &config.Config{Targets: allTargetNames()}) {
		t.Error("a truncated ledger opens the first-sync picker")
	}
}

func TestWhy_NamesATruncatedLedger(t *testing.T) {
	truncateLedger(t)

	err := whyNotTrackedError("CLAUDE.md", ".")

	if !strings.Contains(err.Error(), "does not parse") {
		t.Errorf("why does not name the truncated ledger: %v", err)
	}
}

func TestSyncPlanCheckJSON_ListsATruncatedLedger(t *testing.T) {
	truncateLedger(t)

	out, err := runCLI(t, "sync", "-t", "claude", "--plan", "--check", "--json")

	if err == nil || !strings.Contains(out, "does not parse") {
		t.Errorf("plan check JSON leaves out the truncated ledger: %v\n%s", err, out)
	}
}

// Git does not hold the ledger, so a check against it only warns about
// the working tree's copy.
func TestSyncCheckAgainst_WarnsAboutATruncatedLedger(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/rules/r1.md", "---\nname: r1\n---\nrule body\n")
	if out, err := runCLI(t, "sync", "-t", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	data := readFile(t, stateFilePath("."))
	mustWriteFile(t, stateFilePath("."), data[:len(data)/2])

	out, err := runCLI(t, "sync", "-t", "claude", "--check", "--against", "HEAD")

	if err != nil {
		t.Errorf("check against HEAD failed on the working tree's ledger: %v\n%s", err, out)
	}
	if !strings.Contains(out, "does not parse") {
		t.Errorf("check against HEAD did not warn about the truncated ledger:\n%s", out)
	}
}
