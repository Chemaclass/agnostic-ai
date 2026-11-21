package mailbox

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func TestValidateSend_RefusesCredentialsInEveryField(t *testing.T) {
	const secret = "api_key=example-secret-sentinel"
	for field, inject := range map[string]func(*conversations.Ref, *SendRequest){
		"source tool": func(from *conversations.Ref, _ *SendRequest) { from.Tool = secret },
		"source id":   func(from *conversations.Ref, _ *SendRequest) { from.ID = secret },
		"target tool": func(_ *conversations.Ref, r *SendRequest) { r.To.Tool = secret },
		"target id":   func(_ *conversations.Ref, r *SendRequest) { r.To.ID = secret },
		"type":        func(_ *conversations.Ref, r *SendRequest) { r.Type = secret },
		"thread":      func(_ *conversations.Ref, r *SendRequest) { r.Thread = secret },
		"body":        func(_ *conversations.Ref, r *SendRequest) { r.Body = secret },
	} {
		t.Run(field, func(t *testing.T) {
			from := claude
			request := SendRequest{To: codex, Type: Finding, Body: "I found a solution."}
			inject(&from, &request)
			refused := ValidateSend(from, request)
			if refused == nil || refused.Guard != "secret" {
				t.Errorf("validation = %v, want secret guard", refused)
			}
		})
	}
}

func TestStore_RefusesUnshippedMessageTypesAndOversizeText(t *testing.T) {
	for _, test := range []struct{ name, kind, body, guard string }{
		{"handoffs", "handoff", "Do this next.", "format"},
		{"a body on done", Done, "Goodbye.", "format"},
		{"a body over 4000 characters", Finding, strings.Repeat("a", 4001), "size"},
		{"an encoded line over 5000 bytes", Finding, strings.Repeat("<", 1000), "size"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			_, err := s.Send(context.Background(), claude, SendRequest{To: codex, Type: test.kind, Body: test.body})
			requireGuard(t, err, test.guard)
			list, err := s.Threads(context.Background())
			if err != nil || len(list.Threads) != 0 {
				t.Errorf("threads after refusal = %v, %v", list, err)
			}
		})
	}
}

func TestStore_RefusesNewContentAtTheCapAndAcceptsDone(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	request := SendRequest{To: codex, Type: Finding, Body: "Use the scanner.", Thread: first.Thread}
	for range 7 {
		if _, err := s.Send(ctx, claude, request); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Send(ctx, claude, request)
	requireGuard(t, err, "cap")
	done, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Done, Thread: first.Thread})
	if err != nil || done.N != 9 {
		t.Fatalf("done = %+v, %v", done, err)
	}
	_, err = s.Send(ctx, claude, request)
	requireGuard(t, err, "closed")

	thread := onlyThread(t, s)
	if thread.Status != Closing || len(thread.Refusals) != 2 || thread.Refusals[0].Guard != "cap" || thread.Refusals[0].Reason == "" {
		t.Errorf("thread after done = %+v", thread)
	}
}

func TestStore_RefusesAReplyFromOutsideTheThread(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	missing := conversations.Ref{Tool: conversations.ToolClaude, ID: "77998a87-7f00-4c06-bc0c-b5fbf693eb76"}
	_, err := s.Send(context.Background(), missing, SendRequest{To: codex, Type: Finding, Thread: first.Thread, Body: "I have an answer."})
	requireGuard(t, err, "not-in-thread")
	refusals := onlyThread(t, s).Refusals
	if len(refusals) != 1 || refusals[0].Guard != "not-in-thread" || refusals[0].From != nil {
		t.Errorf("thread should record the refusal without the unrelated session: %+v", refusals)
	}
}

func TestStore_ReviewRequestAcceptsOnlyResolvedCodeRefs(t *testing.T) {
	repo, base := newReviewRepo(t)
	head := reviewCommit(t, repo, "changed.txt")
	root := reviewGitFixture(t, repo, "rev-parse", "--show-toplevel")
	s := Store{Dir: t.TempDir()}
	request := SendRequest{To: codex, Type: ReviewRequest, ReviewFolder: repo, Refs: &ReviewInput{
		Worktree: root, Base: base, Commit: head,
	}}
	for _, example := range []struct {
		name  string
		guard string
		edit  func(*SendRequest)
	}{
		{"body", "echo", func(r *SendRequest) { r.Body = "I already approve this." }},
		{"another worktree", "refs", func(r *SendRequest) { r.Refs.Worktree = t.TempDir() }},
		{"missing base", "refs", func(r *SendRequest) { r.Refs.Base = "" }},
		{"missing refs", "refs", func(r *SendRequest) { r.Refs = nil }},
		{"changed paths", "refs", func(r *SendRequest) { r.Refs.Paths = []string{"elsewhere.txt"} }},
		{"explicit empty paths", "refs", func(r *SendRequest) { r.Refs.Paths = []string{} }},
		{"credential in a path", "secret", func(r *SendRequest) { r.Refs.Paths = []string{"api_key=example-secret-sentinel"} }},
		{"credential in model", "secret", func(r *SendRequest) { r.Models = &ModelInput{From: "api_key=example-secret-sentinel"} }},
		{"oversized path", "size", func(r *SendRequest) { r.Refs.Paths = []string{strings.Repeat("x", 401)} }},
		{"non-Unicode path", "refs", func(r *SendRequest) { r.Refs.Paths = []string{"invalid-\xff"} }},
	} {
		t.Run(example.name, func(t *testing.T) {
			refs := *request.Refs
			req := request
			req.Refs = &refs
			example.edit(&req)
			_, err := s.Send(context.Background(), claude, req)
			requireGuard(t, err, example.guard)
		})
	}
	request.Refs.Paths = []string{"changed.txt"}
	request.Models = &ModelInput{From: "Model A", To: "Model B"}
	receipt, err := s.Send(context.Background(), claude, request)
	if err != nil {
		t.Fatalf("send resolved review = %v", err)
	}
	thread := onlyThread(t, s)
	message := thread.Messages[0]
	if receipt.State != Waiting || message.Type != ReviewRequest || message.Body != "" || message.Refs == nil ||
		message.Refs.Commit != head || message.Refs.Base != base || !slices.Equal(message.Refs.Paths, []string{"changed.txt"}) {
		t.Errorf("stored review = %+v, receipt = %+v", message, receipt)
	}
	if len(thread.Models) != 2 || thread.Models["claude:"+claude.ID] != (Model{Name: "Model A", StatedBy: "human"}) ||
		thread.Models["codex:"+codex.ID] != (Model{Name: "Model B", StatedBy: "human"}) {
		t.Errorf("stored model statements = %+v", thread.Models)
	}
}

func TestStore_ReviewRequestCountsTowardCapAndObeysThreadState(t *testing.T) {
	ctx := context.Background()
	repo, base := newReviewRepo(t)
	reviewCommit(t, repo, "changed.txt")
	root := reviewGitFixture(t, repo, "rev-parse", "--show-toplevel")
	s := Store{Dir: t.TempDir()}
	request := SendRequest{
		To: codex, Type: ReviewRequest, ReviewFolder: repo,
		Refs: &ReviewInput{Worktree: root, Base: base},
	}
	receipt, err := s.Send(ctx, claude, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Thread = receipt.Thread
	if err := s.Change(ctx, receipt.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(ctx, claude, request)
	requireGuard(t, err, "paused")
	if err := s.Change(ctx, receipt.Thread, "resume"); err != nil {
		t.Fatal(err)
	}
	for range 7 {
		sendQuestion(t, s, receipt.Thread)
	}
	_, err = s.Send(ctx, claude, request)
	requireGuard(t, err, "cap")
	if _, err := s.Send(ctx, claude, SendRequest{To: codex, Type: Done, Thread: receipt.Thread}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(ctx, claude, request)
	requireGuard(t, err, "closed")
}
