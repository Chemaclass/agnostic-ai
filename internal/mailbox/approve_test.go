package mailbox

import (
	"testing"
	"time"
)

func sendFromAgent(t *testing.T, s Store, req SendRequest) Receipt {
	t.Helper()
	receipt, err := s.SendFromAgent(t.Context(), claude, req, "")
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func agentQuestion(thread string) SendRequest {
	return SendRequest{To: codex, Type: Question, Body: "Does the lock cover claims?", Thread: thread}
}

func TestApprove_HoldsAnAgentMessageUntilApprovedOnce(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	held := sendFromAgent(t, s, agentQuestion(""))
	if held.State != Held {
		t.Fatalf("receipt state = %q, want held", held.State)
	}
	m := onlyThread(t, s).Messages[0]
	if m.State != Held || m.From.By != "agent" || !m.QueuedAt.IsZero() {
		t.Errorf("held message = %+v", m)
	}
	if err := s.Approve(ctx, held.Thread, held.N, "approve"); err != nil {
		t.Fatal(err)
	}
	thread := onlyThread(t, s)
	if got := thread.Messages[0]; got.State != Waiting || got.QueuedAt.IsZero() {
		t.Errorf("approved message = %+v", got)
	}
	if err := s.Approve(ctx, held.Thread, held.N, "approve"); err != nil {
		t.Errorf("second approve = %v", err)
	}
	again := onlyThread(t, s)
	if len(again.Events) != len(thread.Events) || !again.Messages[0].QueuedAt.Equal(thread.Messages[0].QueuedAt) {
		t.Errorf("second approve changed the thread: %+v", again)
	}
	if last := thread.Events[len(thread.Events)-1]; last.Action != "approve" || last.By != "human" || last.N != 1 {
		t.Errorf("approve event = %+v", last)
	}
}

func TestApprove_OffMeansAgentMessagesQueueAtOnceAndHumanOnesNeverWait(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.SaveSettings(t.Context(), Settings{ApproveAgentSends: false}); err != nil {
		t.Fatal(err)
	}
	if got := sendFromAgent(t, s, agentQuestion("")); got.State != Waiting {
		t.Errorf("agent send with approval off = %q", got.State)
	}
	s2 := Store{Dir: t.TempDir()}
	if got := sendQuestion(t, s2, ""); got.State != Waiting {
		t.Errorf("human send = %q", got.State)
	}
}

func TestApprove_RefusedWhilePausedAndAfterStop(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	held := sendFromAgent(t, s, agentQuestion(first.Thread))
	if err := s.Change(ctx, first.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.Approve(ctx, held.Thread, held.N, "approve"), "paused")
	if got := onlyThread(t, s).Messages[1].State; got != Held {
		t.Errorf("message after paused approve = %q", got)
	}
	if err := s.Change(ctx, first.Thread, "stop"); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Messages[1].State; got != Canceled {
		t.Errorf("held message after stop = %q", got)
	}
	requireGuard(t, s.Approve(ctx, held.Thread, held.N, "approve"), "closed")
}

func TestApprove_RejectRecordsTheGuardAndFreesTheCap(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	for range 6 {
		sendQuestion(t, s, first.Thread)
	}
	held := sendFromAgent(t, s, agentQuestion(first.Thread))
	_, err := s.Send(ctx, claude, agentQuestion(first.Thread))
	requireGuard(t, err, "cap")
	if err := s.Approve(ctx, held.Thread, held.N, "reject"); err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(ctx, held.Thread, held.N, "reject"); err != nil {
		t.Errorf("second reject = %v", err)
	}
	requireGuard(t, s.Approve(ctx, held.Thread, held.N, "approve"), "rejected")
	thread := onlyThread(t, s)
	if thread.Messages[held.N-1].State != Rejected {
		t.Errorf("rejected message = %+v", thread.Messages[held.N-1])
	}
	last := thread.Refusals[len(thread.Refusals)-1]
	if last.Guard != "rejected" || last.Reason == "" || last.Type != Question || last.From == nil || *last.From != claude {
		t.Errorf("reject refusal = %+v", last)
	}
	if _, err := s.Send(ctx, claude, agentQuestion(first.Thread)); err != nil {
		t.Errorf("send after reject = %v", err)
	}
}

func TestApprove_AHeldDoneClosesOnlyWhenApproved(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	done := sendFromAgent(t, s, SendRequest{To: codex, Type: Done, Thread: first.Thread})
	if got := onlyThread(t, s).Status; got != Open {
		t.Errorf("status with held done = %q", got)
	}
	if err := s.Approve(ctx, done.Thread, done.N, "approve"); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Status; got != Closing {
		t.Errorf("status after approving done = %q", got)
	}
}

func TestApprove_ClosingThreadClosesWhenTheLastHeldMessageIsRejected(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	held := sendFromAgent(t, s, agentQuestion(first.Thread))
	if err := s.ConfirmStage(ctx, first.Thread, first.N, "copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Done, Thread: first.Thread}); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmStage(ctx, first.Thread, 3, "copy"); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Status; got != Closing {
		t.Fatalf("status with a held message left = %q", got)
	}
	if err := s.Approve(ctx, held.Thread, held.N, "reject"); err != nil {
		t.Fatal(err)
	}
	if got := onlyThread(t, s).Status; got != Closed {
		t.Errorf("status after the last held message = %q", got)
	}
}

func TestApprove_HandoffChecksOwnershipAgainAndCannotBeAcceptedWhileHeld(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(ctx, claude, Gives{root, []string{"scope"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	gives := Gives{root, []string{"scope/a.go"}}
	held, err := s.SendFromAgent(ctx, claude, SendRequest{To: codex, Type: Handoff, Body: "Take it.", Resume: testResume(root), Gives: &gives}, root)
	if err != nil || held.State != Held {
		t.Fatalf("held handoff = %+v, %v", held, err)
	}
	requireGuard(t, s.AnswerHandoff(ctx, held.Thread, held.N, "accept"), "format")
	if err := s.Release(ctx, claude, Gives{root, []string{"scope"}}); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.Approve(ctx, held.Thread, held.N, "approve"), "ownership")
	if got := onlyThread(t, s).Messages[0].State; got != Held {
		t.Errorf("handoff after failed approve = %q", got)
	}
}

func TestApprove_RejectsUnknownActionsAndApprovedMessages(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	requireGuard(t, s.Approve(ctx, first.Thread, first.N, "reject"), "format")
	requireGuard(t, s.Approve(ctx, first.Thread, first.N, "maybe"), "format")
	requireGuard(t, s.Approve(ctx, first.Thread, 9, "approve"), "not-in-thread")
	if err := s.Approve(ctx, first.Thread, first.N, "approve"); err != nil {
		t.Errorf("approving a waiting message = %v", err)
	}
}

func TestQueue_PutsAStagedMessageBackForDelivery(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	if err := s.ConfirmStage(ctx, first.Thread, first.N, "copy"); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC()
	if err := s.Queue(ctx, first.Thread, first.N); err != nil {
		t.Fatal(err)
	}
	thread := onlyThread(t, s)
	if m := thread.Messages[0]; m.State != Waiting || m.QueuedAt.Before(before) {
		t.Errorf("queued message = %+v", m)
	}
	if err := s.Queue(ctx, first.Thread, first.N); err != nil {
		t.Errorf("queueing a waiting message = %v", err)
	}
	if got := len(onlyThread(t, s).Events); got != len(thread.Events) {
		t.Errorf("second queue added events: %d", got)
	}
	held := sendFromAgent(t, s, agentQuestion(first.Thread))
	requireGuard(t, s.Queue(ctx, held.Thread, held.N), "format")
	if err := s.ConfirmStage(ctx, first.Thread, first.N, "copy"); err != nil {
		t.Fatal(err)
	}
	if err := s.Change(ctx, first.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.Queue(ctx, first.Thread, first.N), "paused")
}

func TestApprove_AHeldOrRejectedHandoffCannotBeAnswered(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(ctx, claude, Gives{root, []string{"scope"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	gives := Gives{root, []string{"scope/a.go"}}
	held, err := s.SendFromAgent(ctx, claude, SendRequest{To: codex, Type: Handoff, Body: "Take it.", Resume: testResume(root), Gives: &gives}, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"accept", "decline"} {
		requireGuard(t, s.AnswerHandoff(ctx, held.Thread, held.N, action), "format")
	}
	if err := s.Approve(ctx, held.Thread, held.N, "reject"); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.AnswerHandoff(ctx, held.Thread, held.N, "decline"), "rejected")
	if got := onlyThread(t, s).Messages[0].Handoff; got != Offered {
		t.Errorf("handoff answer = %q", got)
	}
}

func TestAnswerHandoff_RefusesAnAnswerForATaskWithoutOfferedPaths(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	receipt, err := s.Send(t.Context(), claude, SendRequest{To: codex, Type: Handoff, Resume: testResume(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.AnswerHandoff(t.Context(), receipt.Thread, receipt.N, "accept"), "format")
	requireGuard(t, s.AnswerHandoff(t.Context(), receipt.Thread, receipt.N, "decline"), "format")
}
