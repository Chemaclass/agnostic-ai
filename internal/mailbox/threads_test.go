package mailbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestStore_PauseResumeAllowMoreAndStopAreRecorded(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	if err := s.Change(ctx, first.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Send(ctx, codex, SendRequest{To: claude, Type: Finding, Thread: first.Thread, Body: "Use the scanner."})
	requireGuard(t, err, "paused")
	_, err = s.PrepareStage(ctx, first.Thread, first.N)
	requireGuard(t, err, "paused")
	if err := s.Change(ctx, first.Thread, "resume"); err != nil {
		t.Fatal(err)
	}
	for range 7 {
		sendQuestion(t, s, first.Thread)
	}
	for _, action := range []string{"raise-cap", "raise-cap", "stop"} {
		if err := s.Change(ctx, first.Thread, action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	thread := onlyThread(t, s)
	if thread.Status != Closed || thread.Cap != 16 || thread.Messages[0].State != Canceled {
		t.Errorf("stopped thread = %+v", thread)
	}
	_, err = s.Send(ctx, codex, SendRequest{To: claude, Type: Finding, Thread: first.Thread, Body: "Use the scanner."})
	requireGuard(t, err, "closed")
	var events []string
	for _, event := range thread.Events {
		events = append(events, event.Action)
		if event.By != "human" || event.At.IsZero() {
			t.Errorf("event = %+v", event)
		}
	}
	want := append([]string{"send", "pause", "resume"}, slices.Repeat([]string{"send"}, 7)...)
	want = append(want, "raise-cap", "stop")
	if !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
}

func TestStore_PreparingCopyKeepsItWaitingAndConfirmingDrainsTheThread(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	done, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Done, Thread: first.Thread})
	if err != nil {
		t.Fatal(err)
	}
	prepare, err := s.PrepareStage(ctx, first.Thread, first.N)
	if err != nil || prepare.To != codex || prepare.Text == "" {
		t.Fatalf("prepared message = %+v, %v", prepare, err)
	}
	if got := onlyThread(t, s).Messages[0].State; got != Waiting {
		t.Errorf("preparing changed the state to %q", got)
	}
	if err := s.ConfirmStage(ctx, first.Thread, first.N, "copy"); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Status; got != Closing {
		t.Errorf("thread status with done still waiting = %q", got)
	}
	if err := s.ConfirmStage(ctx, first.Thread, done.N, "type"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmStage(ctx, first.Thread, done.N, "type"); err != nil {
		t.Errorf("retrying confirmation = %v", err)
	}
	thread := onlyThread(t, s)
	if thread.Status != Closed || thread.Messages[0].State != Staged || thread.Messages[1].State != Staged || len(thread.Events) != 4 {
		t.Errorf("drained thread = %+v", thread)
	}
}

func TestStore_TypingSerializesAcrossStoreInstances(t *testing.T) {
	ctx := context.Background()
	firstStore := Store{Dir: t.TempDir()}
	secondStore := Store{Dir: firstStore.Dir}
	first := sendQuestion(t, firstStore, "")
	entered := make(chan struct{})
	release := make(chan struct{})
	type result struct {
		typed bool
		err   error
	}
	done := make(chan result, 1)
	go func() {
		typed, err := firstStore.TypeStage(ctx, first.Thread, first.N, func(stage Stage) bool {
			if stage.To != codex || stage.Text == "" {
				t.Errorf("first dispatch received %+v", stage)
			}
			close(entered)
			<-release
			return true
		})
		done <- result{typed, err}
	}()
	select {
	case <-entered:
	case got := <-done:
		t.Fatalf("first typing did not dispatch: %+v", got)
	}

	wait, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	typed, err := secondStore.TypeStage(wait, first.Thread, first.N, func(Stage) bool {
		t.Error("second store dispatched while first call still had the message")
		return true
	})
	if typed || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("competing typing = %t, %v, want caller deadline", typed, err)
	}
	if err := secondStore.Change(ctx, first.Thread, "pause"); !errors.Is(err, ErrBusy) {
		t.Errorf("pause during typing = %v, want mailbox busy", err)
	}
	close(release)
	if got := <-done; !got.typed || got.err != nil {
		t.Fatalf("first typing = %+v", got)
	}

	typed, err = secondStore.TypeStage(ctx, first.Thread, first.N, func(Stage) bool {
		t.Error("a retry dispatched the staged message again")
		return true
	})
	if typed {
		t.Error("retry reported that typing occurred")
	}
	requireGuard(t, err, "format")
	thread := onlyThread(t, secondStore)
	if thread.Messages[0].State != Staged || len(thread.Events) != 2 || thread.Events[1].Action != "type" {
		t.Errorf("typing should record one receipt: %+v", thread)
	}
}

func TestStore_FailedTypingKeepsTheMessageWaiting(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	typed, err := s.TypeStage(context.Background(), first.Thread, first.N, func(Stage) bool { return false })
	if typed || err != nil {
		t.Errorf("failed typing = %t, %v", typed, err)
	}
	thread := onlyThread(t, s)
	if thread.Messages[0].State != Waiting || len(thread.Events) != 1 {
		t.Errorf("failed typing changed the thread: %+v", thread)
	}
}

func TestStore_ReportsTypingEvenWhenTheReceiptCannotBeSaved(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	path := s.threadPath(first.Thread)
	typed, err := s.TypeStage(context.Background(), first.Thread, first.N, func(Stage) bool {
		if err := os.Rename(path, filepath.Join(s.Dir, "previous.json")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if !typed || err == nil {
		t.Errorf("typing with failed receipt = %t, %v", typed, err)
	}
}
