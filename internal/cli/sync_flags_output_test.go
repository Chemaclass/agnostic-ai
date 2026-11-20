package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_RejectsANegativeJobCount(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	out, err := runCLI(t, "sync", "--jobs", "-1", "--gitignore=off")
	if err == nil || !strings.Contains(err.Error(), "--jobs") {
		t.Errorf("sync --jobs -1: %v\n%s", err, out)
	}
	if got := errs.CodeOf(err); got != errs.CodeFlagConflict {
		t.Errorf("sync --jobs -1 code = %v, want %v", got, errs.CodeFlagConflict)
	}
}

func TestWatchSync_PrintsOneSummaryAtStart(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	buf := captureWatchOutput(t)
	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 10*time.Millisecond, ".", []string{"claude"}, watchOptions{gitignoreFlag: "off", jobs: 1}, false)
	}()
	waitForOutput(t, buf, "watching", 10*time.Second)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "up to date"); n != 1 {
		t.Errorf("start printed %d summaries, want 1:\n%s", n, buf.String())
	}
}
