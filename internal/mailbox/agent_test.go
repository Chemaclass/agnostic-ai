package mailbox

import (
	"strings"
	"testing"
)

func TestSendFromAgent_StoresTheStatedModelWithinItsLimits(t *testing.T) {
	for _, test := range []struct{ name, model, guard string }{
		{"a model name of 80 characters", strings.Repeat("a", 80), ""},
		{"names with dots, slashes, colons and spaces", "openai/gpt-5.5 high:v2_1", ""},
		{"a model name of 81 characters", strings.Repeat("a", 81), "size"},
		{"a character outside the set", "gpt<5>", "format"},
		{"a line break", "gpt\n5", "format"},
		{"a credential", "api_key=example-secret-sentinel", "secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			req := agentQuestion("")
			req.Model = test.model
			_, err := s.SendFromAgent(t.Context(), claude, req, "")
			if test.guard != "" {
				requireGuard(t, err, test.guard)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := onlyThread(t, s).Models[string(claude.Tool)+":"+claude.ID]
			if got.Name != test.model || got.StatedBy != "agent" {
				t.Errorf("stated model = %+v", got)
			}
		})
	}
}

func TestSend_TheModelFieldIsForAgentsAndModelsForHumans(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	req := agentQuestion("")
	req.Model = "gpt-5.5"
	_, err := s.Send(t.Context(), claude, req)
	requireGuard(t, err, "format")

	req = agentQuestion("")
	req.Models = &ModelInput{From: "gpt-5.5"}
	_, err = s.SendFromAgent(t.Context(), claude, req, "")
	requireGuard(t, err, "format")
}

func TestSendFromAgent_HeldMessagesCountTowardTheCap(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendFromAgent(t, s, agentQuestion(""))
	for range threadCap - 1 {
		if got := sendFromAgent(t, s, agentQuestion(first.Thread)); got.State != Held {
			t.Fatalf("receipt = %+v, want held", got)
		}
	}
	_, err := s.SendFromAgent(t.Context(), claude, agentQuestion(first.Thread), "")
	requireGuard(t, err, "cap")
}

func TestSendFromAgent_RecordsEventsAsTheAgent(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.SaveSettings(t.Context(), Settings{ApproveAgentSends: false}); err != nil {
		t.Fatal(err)
	}
	first := sendFromAgent(t, s, agentQuestion(""))
	sendFromAgent(t, s, SendRequest{To: codex, Type: Done, Thread: first.Thread})
	events := onlyThread(t, s).Events
	if len(events) != 2 || events[0].Action != "send" || events[1].Action != "close" {
		t.Fatalf("events = %+v", events)
	}
	for _, event := range events {
		if event.By != "agent" {
			t.Errorf("event %+v, want by agent", event)
		}
	}
}

func TestSendFromAgent_RefusesASecondDoneWhileOneWaitsForApproval(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendFromAgent(t, s, agentQuestion(""))
	sendFromAgent(t, s, SendRequest{To: codex, Type: Done, Thread: first.Thread})
	_, err := s.SendFromAgent(t.Context(), claude, SendRequest{To: codex, Type: Done, Thread: first.Thread}, "")
	requireGuard(t, err, "closed")
}

func TestPeer_NamesTheOtherSessionOrRefusesAThirdOne(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	if peer, err := s.Peer(t.Context(), first.Thread, codex); err != nil || peer != claude {
		t.Errorf("peer of codex = %+v, %v", peer, err)
	}
	_, err := s.Peer(t.Context(), first.Thread, mailThird)
	requireGuard(t, err, "not-in-thread")
}

func TestAnswerHandoffFromAgent_OnlyTheOfferedSessionAnswers(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")

	err := s.AnswerHandoffFromAgent(t.Context(), mailThird, offer.Thread, offer.N, "accept")
	requireGuard(t, err, "not-in-thread")
	err = s.AnswerHandoffFromAgent(t.Context(), claude, offer.Thread, offer.N, "accept")
	requireGuard(t, err, "ownership")
	expectedOwner(t, s, root, "scope/a.go", claude)

	if err := s.AnswerHandoffFromAgent(t.Context(), codex, offer.Thread, offer.N, "accept"); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/a.go", codex)
	thread := onlyThread(t, s)
	last := thread.Events[len(thread.Events)-1]
	if last.Action != "accept" || last.By != "agent" || last.N != offer.N {
		t.Errorf("accept event = %+v", last)
	}
	if len(thread.Refusals) != 2 {
		t.Errorf("refusals = %+v, want both recorded", thread.Refusals)
	}
}

func TestAnswerHandoffFromAgent_DeclineIsRecordedAsTheAgent(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	if err := s.AnswerHandoffFromAgent(t.Context(), codex, offer.Thread, offer.N, "decline"); err != nil {
		t.Fatal(err)
	}
	thread := onlyThread(t, s)
	last := thread.Events[len(thread.Events)-1]
	if thread.Messages[0].Handoff != Declined || last.Action != "decline" || last.By != "agent" {
		t.Errorf("declined thread = %+v", thread)
	}
	expectedOwner(t, s, root, "scope/a.go", claude)
}

func TestThreadsFor_ShowsOnlyContentTheSessionMayRead(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	sent := sendQuestion(t, s, "")
	// codex's held reply: the sender sees it, the reader does not.
	reply := SendRequest{To: claude, Type: Finding, Body: "Held answer.", Thread: sent.Thread}
	if _, err := s.SendFromAgent(t.Context(), codex, reply, ""); err != nil {
		t.Fatal(err)
	}

	mine, err := s.ThreadFor(t.Context(), sent.Thread, claude)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Messages) != 2 || mine.Messages[0].Body != "How does this work?" ||
		mine.Messages[1].Body != "" || mine.Messages[1].State != Held || mine.Messages[1].Type != Finding {
		t.Errorf("claude's view = %+v", mine.Messages)
	}

	theirs, err := s.ThreadFor(t.Context(), sent.Thread, codex)
	if err != nil {
		t.Fatal(err)
	}
	// The question waits for delivery, so codex has not been shown it yet.
	if theirs.Messages[0].Body != "" || theirs.Messages[1].Body != "Held answer." {
		t.Errorf("codex's view = %+v", theirs.Messages)
	}

	list, err := s.ThreadsFor(t.Context(), mailThird)
	if err != nil || len(list.Threads) != 0 {
		t.Errorf("outsider list = %+v, %v", list, err)
	}
	_, err = s.ThreadFor(t.Context(), sent.Thread, mailThird)
	requireGuard(t, err, "not-in-thread")
}

func TestThreadsFor_ShowsDeliveredContentToTheReader(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	sent := sendQuestion(t, s, "")
	delivery, err := s.Reserve(t.Context(), codex)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	list, err := s.ThreadsFor(t.Context(), codex)
	if err != nil || len(list.Threads) != 1 || list.Threads[0].ID != sent.Thread ||
		list.Threads[0].Messages[0].Body != "How does this work?" {
		t.Errorf("reader list = %+v, %v", list, err)
	}
}
