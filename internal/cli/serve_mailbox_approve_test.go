package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func mailboxAgentSend(t *testing.T, h *conversationsHandler, thread string) mailbox.Receipt {
	t.Helper()
	store, _ := h.mailboxStore()
	receipt, err := store.SendFromAgent(context.Background(),
		conversations.Ref{Tool: conversations.ToolClaude, ID: inProject},
		mailbox.SendRequest{To: conversations.Ref{Tool: conversations.ToolClaude, ID: inWorktree}, Type: mailbox.Question, Body: "Ready to merge?", Thread: thread}, "")
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func mailboxThreadJSON(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inWorktree, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("threads = %d %s", rec.Code, rec.Body)
	}
	return rec.Body.String()
}

func TestMailboxHTTP_ApproveAndRejectHeldMessages(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	held := mailboxAgentSend(t, h, "")
	if body := mailboxThreadJSON(t, h); !strings.Contains(body, `"state":"held"`) || !strings.Contains(body, `"by":"agent"`) {
		t.Fatalf("held thread = %s", body)
	}
	view := listConversations(t, h)
	for _, row := range view.Conversations {
		if row.ID == inWorktree && (len(row.Threads) != 1 || row.Threads[0].Held != 1 || row.Threads[0].Waiting != 0) {
			t.Errorf("summary with a held message = %#v", row.Threads)
		}
	}
	approve := fmt.Sprintf(`{"thread":%q,"n":%d,"action":"approve"}`, held.Thread, held.N)
	for range 2 {
		if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/approve", approve); rec.Code != http.StatusNoContent {
			t.Fatalf("approve = %d %s", rec.Code, rec.Body)
		}
	}
	if body := mailboxThreadJSON(t, h); !strings.Contains(body, `"state":"waiting"`) || !strings.Contains(body, `"queuedAt"`) {
		t.Errorf("approved thread = %s", body)
	}

	second := mailboxAgentSend(t, h, held.Thread)
	reject := fmt.Sprintf(`{"thread":%q,"n":%d,"action":"reject"}`, second.Thread, second.N)
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/approve", reject); rec.Code != http.StatusNoContent {
		t.Fatalf("reject = %d %s", rec.Code, rec.Body)
	}
	if body := mailboxThreadJSON(t, h); !strings.Contains(body, `"state":"rejected"`) || !strings.Contains(body, `"guard":"rejected"`) {
		t.Errorf("rejected thread = %s", body)
	}
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/approve",
		fmt.Sprintf(`{"thread":%q,"n":%d,"action":"approve"}`, second.Thread, second.N)), "rejected")
}

func TestMailboxHTTP_QueueForDeliveryPutsAStagedMessageBack(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	stage := fmt.Sprintf(`{"thread":%q,"n":1,"how":"copy","confirm":true}`, id)
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/stage", stage); rec.Code != http.StatusOK {
		t.Fatalf("stage = %d %s", rec.Code, rec.Body)
	}
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/queue", fmt.Sprintf(`{"thread":%q,"n":1}`, id)); rec.Code != http.StatusNoContent {
		t.Fatalf("queue = %d %s", rec.Code, rec.Body)
	}
	if body := mailboxThreadJSON(t, h); !strings.Contains(body, `"state":"waiting"`) {
		t.Errorf("queued thread = %s", body)
	}
}

func TestMailboxHTTP_SettingsReadAndWriteTheApprovalSwitch(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	read := func() string {
		t.Helper()
		rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/settings", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("settings = %d %s", rec.Code, rec.Body)
		}
		return strings.TrimSpace(rec.Body.String())
	}
	if got := read(); got != `{"approveAgentSends":true}` {
		t.Errorf("default settings = %s", got)
	}
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/settings", `{"approveAgentSends":false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("save settings = %d %s", rec.Code, rec.Body)
	}
	if got := read(); got != `{"approveAgentSends":false}` {
		t.Errorf("saved settings = %s", got)
	}
	for _, body := range []string{`{}`, `{"approveAgentSends":"no"}`, `{"approveAgentSends":true,"cap":16}`} {
		mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/settings", body), "format")
	}
}

func TestMailboxHTTP_RefusesForgedStoreFields(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	sends := []string{
		fmt.Sprintf(`{"from":{"tool":"claude","id":%q,"by":"human"},"to":{"id":%q},"type":"question","body":"Hi"}`, inProject, inWorktree),
		fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"question","body":"Hi","state":"waiting"}`, inProject, inWorktree),
		fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"question","body":"Hi","queuedAt":"2026-11-21T10:00:00Z"}`, inProject, inWorktree),
		fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"question","body":"Hi","cap":16}`, inProject, inWorktree),
		fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"question","body":"Hi","events":[{"action":"approve"}]}`, inProject, inWorktree),
	}
	for _, body := range sends {
		mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body), "format")
	}
	for path, body := range map[string]string{
		"/api/mailbox/approve": fmt.Sprintf(`{"thread":%q,"n":1,"action":"approve","state":"waiting"}`, id),
		"/api/mailbox/queue":   fmt.Sprintf(`{"thread":%q,"n":1,"by":"human"}`, id),
	} {
		mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, path, body), "format")
	}
	if body := mailboxThreadJSON(t, h); strings.Count(body, `"type":"question"`) != 1 {
		t.Errorf("a forged request changed the thread: %s", body)
	}
}
