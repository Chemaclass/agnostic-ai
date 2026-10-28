package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// countLedgerReads counts the reads of the working directory's ledger.
func countLedgerReads(t *testing.T) *int {
	t.Helper()
	reads := 0
	want := filepath.Clean(stateFilePath("."))
	original := readStateBytes
	readStateBytes = func(path string) ([]byte, error) {
		if filepath.Clean(path) == want {
			reads++
		}
		return original(path)
	}
	t.Cleanup(func() { readStateBytes = original })
	return &reads
}

func syncedFixture(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, setupFixture(t))
	silence(t)
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestSync_ReadsLedgerOnce(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}, {"sync", "--dry-run"}, {"sync", "--plan"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			syncedFixture(t)
			reads := countLedgerReads(t)

			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}

			if *reads != 1 {
				t.Errorf("%v read the ledger %d times, want 1", args, *reads)
			}
		})
	}
}

func TestSyncCheck_ReadsLedgerOnce(t *testing.T) {
	syncedFixture(t)
	reads := countLedgerReads(t)

	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("sync --check: %v\n%s", err, out)
	}

	if *reads != 1 {
		t.Errorf("sync --check read the ledger %d times, want 1", *reads)
	}
}

func TestSync_ReadsATruncatedLedgerOnceAndWarnsOnce(t *testing.T) {
	truncateLedger(t)
	reads := countLedgerReads(t)

	var err error
	stderr := captureStderr(t, func() {
		_, err = runCLI(t, "sync", "-t", "claude")
	})

	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if *reads != 1 {
		t.Errorf("sync read the ledger %d times, want 1", *reads)
	}
	warning := "! .agnostic-ai/.sync-state does not parse ("
	suffix := "; treating the ledger as missing until a sync writes a new one\n"
	if strings.Count(stderr, warning) != 1 || !strings.Contains(stderr, suffix) {
		t.Errorf("want one warning %q...%q, stderr:\n%s", warning, suffix, stderr)
	}
}

func TestSyncCheck_ReadsATruncatedLedgerOnceAndFails(t *testing.T) {
	truncateLedger(t)
	reads := countLedgerReads(t)

	_, err := runCLI(t, "sync", "-t", "claude", "--check")

	if *reads != 1 {
		t.Errorf("sync --check read the ledger %d times, want 1", *reads)
	}
	if err == nil {
		t.Fatal("check passed on a truncated ledger")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, ".agnostic-ai/.sync-state does not parse (") || !strings.HasSuffix(msg, "); run agnostic-ai sync to rebuild the ledger") {
		t.Errorf("error = %q", msg)
	}
}
