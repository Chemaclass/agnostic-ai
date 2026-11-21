package mailbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func editThread(t *testing.T, s Store, id string, change func(*Thread)) {
	t.Helper()
	thread, err := s.readThread(id)
	if err != nil {
		t.Fatal(err)
	}
	change(&thread)
	if err := s.writeThread(thread); err != nil {
		t.Fatal(err)
	}
}

func threadByID(t *testing.T, s Store, id string) Thread {
	t.Helper()
	thread, err := s.readThread(id)
	if err != nil {
		t.Fatal(err)
	}
	return thread
}

func reserve(t *testing.T, s Store) Delivery {
	t.Helper()
	delivery, err := s.Reserve(t.Context(), codex)
	if err != nil {
		t.Fatal(err)
	}
	return delivery
}

func TestDeliver_PrintsOldestFirstAcrossThreadsAndMarksThemDelivered(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	newer := sendQuestion(t, s, "")
	older := sendQuestion(t, s, "")
	editThread(t, s, older.Thread, func(th *Thread) { th.Messages[0].QueuedAt = th.Messages[0].QueuedAt.Add(-time.Minute) })

	delivery := reserve(t, s)
	if !strings.HasPrefix(delivery.Block, blockOpening) || !strings.HasSuffix(delivery.Block, blockClosing) {
		t.Fatalf("block = %q", delivery.Block)
	}
	if first, second := strings.Index(delivery.Block, older.Thread), strings.Index(delivery.Block, newer.Thread); first < 0 || second < first {
		t.Errorf("block order = %q", delivery.Block)
	}
	if m := threadByID(t, s, older.Thread).Messages[0]; m.State != Reserved || m.ReservedAt == nil {
		t.Errorf("reserved message = %+v", m)
	}
	if err := s.Confirm(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{older.Thread, newer.Thread} {
		m := threadByID(t, s, id).Messages[0]
		if m.State != Delivered || m.DeliveredAt == nil || m.ReservedAt != nil {
			t.Errorf("delivered message = %+v", m)
		}
	}
	if again := reserve(t, s); again.Block != "" {
		t.Errorf("second run printed %q", again.Block)
	}
}

func TestDeliver_SkipsMessagesThatAreNotWaitingAndThreadsThatAreNotOpen(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	mixed := sendQuestion(t, s, "")
	staged := sendQuestion(t, s, mixed.Thread)
	if err := s.ConfirmStage(ctx, mixed.Thread, staged.N, "copy"); err != nil {
		t.Fatal(err)
	}
	held := sendFromAgent(t, s, agentQuestion(mixed.Thread))
	rejected := sendFromAgent(t, s, agentQuestion(mixed.Thread))
	if err := s.Approve(ctx, mixed.Thread, rejected.N, "reject"); err != nil {
		t.Fatal(err)
	}
	toClaude, err := s.Send(ctx, codex, SendRequest{To: claude, Type: Finding, Body: "For the other side.", Thread: mixed.Thread})
	if err != nil {
		t.Fatal(err)
	}
	paused := sendQuestion(t, s, "")
	if err := s.Change(ctx, paused.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	stopped := sendQuestion(t, s, "")
	if err := s.Change(ctx, stopped.Thread, "stop"); err != nil {
		t.Fatal(err)
	}

	delivery := reserve(t, s)
	for _, unwanted := range []string{paused.Thread, stopped.Thread} {
		if strings.Contains(delivery.Block, unwanted) {
			t.Errorf("printed a message of thread %s", unwanted)
		}
	}
	if len(delivery.items) != 1 || delivery.items[0].n != mixed.N {
		t.Errorf("reserved %+v, want only message %d", delivery.items, mixed.N)
	}
	thread := threadByID(t, s, mixed.Thread)
	for n, want := range map[int]string{staged.N: Staged, held.N: Held, rejected.N: Rejected, toClaude.N: Waiting} {
		if got := thread.Messages[n-1].State; got != want {
			t.Errorf("message %d = %q, want %q", n, got, want)
		}
	}
}

func TestDeliver_AClosingThreadDrainsAndThenCloses(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	if _, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Done, Thread: first.Thread}); err != nil {
		t.Fatal(err)
	}
	delivery := reserve(t, s)
	if len(delivery.items) != 2 {
		t.Fatalf("closing thread reserved %+v", delivery.items)
	}
	if err := s.Confirm(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Status; got != Closed {
		t.Errorf("status after drain = %q", got)
	}
}

func TestDeliver_StaysWithinTheBudgetAfterEscapingAndKeepsTheRestForLater(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	bodies := []string{strings.Repeat("界", 1550), strings.Repeat(`<"`, 550) + "\n", strings.Repeat("ü", 2000), strings.Repeat("界", 1550)}
	first := sendQuestion(t, s, "")
	for _, body := range bodies {
		if _, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Finding, Body: body, Thread: first.Thread}); err != nil {
			t.Fatal(err)
		}
	}
	delivered := map[int]bool{}
	for run := 0; run < len(bodies)+1 && len(delivered) < len(bodies)+1; run++ {
		delivery := reserve(t, s)
		if delivery.Block == "" {
			t.Fatalf("run %d printed nothing with %d of %d delivered", run, len(delivered), len(bodies)+1)
		}
		if len(delivery.Block) > deliveryBudget {
			t.Errorf("run %d printed %d bytes", run, len(delivery.Block))
		}
		for _, item := range delivery.items {
			if delivered[item.n] {
				t.Errorf("message %d printed twice", item.n)
			}
			delivered[item.n] = true
		}
		if err := s.Confirm(ctx, delivery); err != nil {
			t.Fatal(err)
		}
	}
	if len(delivered) != len(bodies)+1 {
		t.Errorf("delivered %v", delivered)
	}
}

func TestDeliver_CancelsAnOversizedMessageAndDeliversTheNext(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	second := sendQuestion(t, s, first.Thread)
	editThread(t, s, first.Thread, func(th *Thread) { th.Messages[0].Body = strings.Repeat("<", maxLineBytes) })

	delivery := reserve(t, s)
	if len(delivery.items) != 1 || delivery.items[0].n != second.N {
		t.Fatalf("reserved %+v", delivery.items)
	}
	thread := threadByID(t, s, first.Thread)
	if thread.Messages[0].State != Canceled {
		t.Errorf("oversized message = %q", thread.Messages[0].State)
	}
	last := thread.Events[len(thread.Events)-1]
	if last.By != "hook" || last.Action != "cancel-oversize" || last.N != 1 {
		t.Errorf("oversize event = %+v", last)
	}
}

func TestDeliver_TwoRunsAtOncePrintEachMessageOnce(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	for range 5 {
		sendQuestion(t, s, first.Thread)
	}
	var wg sync.WaitGroup
	results := make([]Delivery, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Go(func() { results[i], errs[i] = Store{Dir: s.Dir}.Reserve(t.Context(), codex) })
	}
	wg.Wait()
	seen := map[int]int{}
	for i, d := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		for _, item := range d.items {
			seen[item.n]++
		}
	}
	if len(seen) != 6 {
		t.Errorf("reserved messages = %v", seen)
	}
	for n, count := range seen {
		if count != 1 {
			t.Errorf("message %d reserved %d times", n, count)
		}
	}
}

func TestDeliver_PutBackLeavesTheMessageWaiting(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	if err := s.PutBack(t.Context(), reserve(t, s)); err != nil {
		t.Fatal(err)
	}
	if m := onlyThread(t, s).Messages[0]; m.State != Waiting || m.ReservedAt != nil {
		t.Errorf("message after put back = %+v", m)
	}
	if again := reserve(t, s); len(again.items) != 1 || again.items[0].thread != first.Thread {
		t.Errorf("next run reserved %+v", again.items)
	}
}

func TestDeliver_AnOldReservationPrintsAgainWithTheSameThreadAndNumber(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	crashed := reserve(t, s)
	if fresh := reserve(t, s); fresh.Block != "" {
		t.Fatalf("a fresh reservation printed again: %q", fresh.Block)
	}
	editThread(t, s, first.Thread, func(th *Thread) {
		old := th.Messages[0].ReservedAt.Add(-reservationAge - time.Second)
		th.Messages[0].ReservedAt = &old
	})
	retry := reserve(t, s)
	if retry.Block != crashed.Block {
		t.Errorf("retry block = %q, want %q", retry.Block, crashed.Block)
	}
	if err := s.Confirm(ctx, crashed); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Messages[0].State; got != Reserved {
		t.Errorf("a stale confirm changed the new reservation to %q", got)
	}
	if err := s.Confirm(ctx, retry); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Messages[0].State; got != Delivered {
		t.Errorf("message after retry = %q", got)
	}
}

func TestDeliver_AFailedSaveAfterPrintingKeepsTheReservation(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	delivery := reserve(t, s)
	path := s.threadPath(first.Thread)
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(t.Context(), delivery); err == nil {
		t.Error("confirm into a broken thread file succeeded")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Messages[0].State; got != Reserved {
		t.Errorf("message after failed save = %q", got)
	}
}

func TestDeliver_MissingEmptyCorruptAndBusyMailboxes(t *testing.T) {
	missing := Store{Dir: filepath.Join(t.TempDir(), "mailbox")}
	if d := reserve(t, missing); d.Block != "" {
		t.Errorf("missing mailbox printed %q", d.Block)
	}
	if _, err := os.Stat(missing.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("reserve created the mailbox: %v", err)
	}

	s := Store{Dir: t.TempDir()}
	if d := reserve(t, s); d.Block != "" {
		t.Errorf("empty mailbox printed %q", d.Block)
	}
	good := sendQuestion(t, s, "")
	if err := os.WriteFile(s.threadPath("th_20260101_00000000"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := reserve(t, s); !strings.Contains(d.Block, good.Thread) {
		t.Errorf("corrupt neighbor blocked delivery: %q", d.Block)
	}

	release, err := (Store{Dir: s.Dir}).lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	if _, err := s.Reserve(t.Context(), codex); !errors.Is(err, ErrBusy) {
		t.Errorf("reserve under a held lock = %v", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("reserve waited %s for the lock", waited)
	}
}
