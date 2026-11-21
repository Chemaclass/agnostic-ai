package mailbox

import (
	"context"
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
		{"review requests", "review_request", "", "format"},
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
