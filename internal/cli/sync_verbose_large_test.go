package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncVerbose_LargeRepeatIsCompactAndStableAcrossJobs(t *testing.T) {
	if got := len(adapters.Names()); got < 31 {
		t.Fatalf("fixture has only %d targets, want at least 31", got)
	}
	run := func(jobs int) string {
		t.Helper()
		dir := t.TempDir()
		writeParallelFixture(t, dir, "")
		testutil.Chdir(t, dir)
		var log, warnings strings.Builder
		prevOut, prevVerbosity := logOut, verbosity
		logOut = &log
		adapters.SetWarner(&warnings)
		defer func() {
			logOut, verbosity = prevOut, prevVerbosity
			adapters.SetWarner(os.Stderr)
		}()
		sync := func(verbose bool) {
			t.Helper()
			root := NewRootCmd("test")
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			args := []string{"sync", "--gitignore", "on", "--jobs", strconv.Itoa(jobs)}
			if verbose {
				args = append(args, "-v")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("sync --jobs %d: %v", jobs, err)
			}
		}
		sync(false)
		log.Reset()
		warnings.Reset()
		sync(true)
		got := log.String()
		if !strings.Contains(got, fmt.Sprintf("%d targets had no writes", len(adapters.Names()))) {
			t.Errorf("repeat sync did not summarize the configured targets:\n%s", got)
		}
		if !strings.Contains(got, "unchanged since last sync") || !strings.Contains(got, "-vv shows them") {
			t.Errorf("repeat sync did not summarize repeated diagnostics:\n%s", got)
		}
		if got := warnings.String(); got != "" {
			t.Errorf("repeat sync replayed unchanged diagnostics:\n%s", got)
		}
		return durationToken.ReplaceAllString(got+"\n"+warnings.String(), "DUR")
	}
	serial := run(1)
	parallel := run(8)
	if serial != parallel {
		t.Errorf("compact output changed with --jobs:\n--- jobs 1 ---\n%s\n--- jobs 8 ---\n%s", serial, parallel)
	}
}
