package cli

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
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

func TestMailboxHTTP_PersonActionsNeedTheCompanionSecret(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	held := mailboxAgentSend(t, h, "")
	at := func(action string) string {
		return fmt.Sprintf(`{"thread":%q,"n":%d,"action":%q}`, held.Thread, held.N, action)
	}
	owner := fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"paths":["src/a.go"]}`, inWorktree, h.root)
	calls := []struct{ path, body string }{
		{"/api/mailbox/approve", at("approve")},
		{"/api/mailbox/approve", at("reject")},
		{"/api/mailbox/handoff", at("accept")},
		{"/api/mailbox/handoff", at("decline")},
		{"/api/mailbox/queue", fmt.Sprintf(`{"thread":%q,"n":%d}`, held.Thread, held.N)},
		{"/api/mailbox/stage", fmt.Sprintf(`{"thread":%q,"n":%d,"how":"copy"}`, held.Thread, held.N)},
		{"/api/mailbox/claim", owner},
		{"/api/mailbox/release", owner},
		{"/api/mailbox/conflict/dismiss", fmt.Sprintf(`{"tool":"claude","id":%q,"conflict":"c1"}`, inWorktree)},
	}
	for _, action := range []string{"pause", "resume", "stop", "raise-cap"} {
		calls = append(calls, struct{ path, body string }{"/api/mailbox/thread", fmt.Sprintf(`{"thread":%q,"action":%q}`, held.Thread, action)})
	}
	before := mailboxThreadJSON(t, h)
	claims := func() string {
		return mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/claims", "").Body.String()
	}
	claimsBefore := claims()
	secret := string(testCompanionSecret)
	for name, as := range map[string]string{"no secret": "", "a wrong secret": strings.Repeat("0", len(secret))} {
		for _, call := range calls {
			if rec := mailboxHTTPAs(t, h, as, http.MethodPost, call.path, call.body); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s with %s = %d %s, want 403", call.path, call.body, name, rec.Code, rec.Body)
			}
		}
	}
	if after := mailboxThreadJSON(t, h); after != before {
		t.Errorf("refused calls changed the thread:\nbefore %s\nafter  %s", before, after)
	}
	if after := claims(); after != claimsBefore {
		t.Errorf("refused calls changed the claims:\nbefore %s\nafter  %s", claimsBefore, after)
	}
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/approve", at("reject")); rec.Code != http.StatusNoContent {
		t.Errorf("reject with the secret = %d %s", rec.Code, rec.Body)
	}

	h.companion = ""
	other := mailboxAgentSend(t, h, held.Thread)
	approveOther := fmt.Sprintf(`{"thread":%q,"n":%d,"action":"approve"}`, other.Thread, other.N)
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/approve", approveOther); rec.Code != http.StatusForbidden {
		t.Errorf("approve on a server started without a secret = %d %s, want 403", rec.Code, rec.Body)
	}
}

func TestMailboxNeedsCompanion_GatesEveryChangeRouteButSendRefsAndSettings(t *testing.T) {
	for path, want := range map[string]bool{
		mailboxSendPath: false, mailboxRefsPath: false, mailboxSettingsPath: false,
		mailboxThreadPath: true, mailboxStagePath: true, mailboxClaimPath: true, mailboxReleasePath: true,
		mailboxDismissPath: true, mailboxHandoffPath: true, mailboxApprovePath: true, mailboxQueuePath: true,
		"/api/mailbox/added-later": true,
	} {
		if got := mailboxNeedsCompanion(path); got != want {
			t.Errorf("mailboxNeedsCompanion(%s) = %t, want %t", path, got, want)
		}
	}
}

func TestMailboxHTTP_SendWithoutTheSecretIsAnAgentSend(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	secret := string(testCompanionSecret)
	for name, as := range map[string]string{"no secret": "", "a wrong secret": strings.Repeat("0", len(secret))} {
		rec := mailboxHTTPAs(t, h, as, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "question", "Ready to merge?", ""))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"held"`) {
			t.Fatalf("send with %s = %d %s, want a held receipt", name, rec.Code, rec.Body)
		}
	}
	if body := mailboxThreadJSON(t, h); !strings.Contains(body, `"by":"agent"`) || strings.Contains(body, `"by":"human"`) {
		t.Errorf("a send without the secret was recorded as human: %s", body)
	}
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"waiting"`) {
		t.Errorf("send with the secret = %d %s, want a waiting receipt", rec.Code, rec.Body)
	}
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
	if rec := mailboxHTTPAs(t, h, "", http.MethodPost, "/api/mailbox/settings", `{"approveAgentSends":false}`); rec.Code != http.StatusForbidden {
		t.Fatalf("turning approval off without the companion secret = %d %s, want 403", rec.Code, rec.Body)
	}
	if got := read(); got != `{"approveAgentSends":true}` {
		t.Errorf("a refused write changed the settings: %s", got)
	}
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/settings", `{"approveAgentSends":false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("save settings with the secret = %d %s", rec.Code, rec.Body)
	}
	if got := read(); got != `{"approveAgentSends":false}` {
		t.Errorf("saved settings = %s", got)
	}
	if rec := mailboxHTTPAs(t, h, "", http.MethodPost, "/api/mailbox/settings", `{"approveAgentSends":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("turning approval on without the secret = %d %s", rec.Code, rec.Body)
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

func TestMailboxHTTP_TypingAStagedMessageNeedsTheSecret(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	writeFile(t, filepath.Join(h.scanner.Stores.ClaudeDir, "sessions", "77.json"), `{"pid":77,"sessionId":"`+inWorktree+`","status":"idle"}`)
	h.scanner.Processes = mailboxTerminalProcesses{runningProcesses: runningProcesses{pid: 77}}
	typed := 0
	h.launcher = &mailboxTerminalLauncher{typeInTab: func(context.Context, string, string, string, bool) (bool, error) {
		typed++
		return true, nil
	}}
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	body := fmt.Sprintf(`{"thread":%q,"n":1,"how":"type"}`, id)
	for _, secret := range []string{"", strings.Repeat("0", len(testCompanionSecret))} {
		if rec := mailboxHTTPAs(t, h, secret, http.MethodPost, "/api/mailbox/stage", body); rec.Code != http.StatusForbidden {
			t.Errorf("type a staged message with secret %q = %d %s, want 403", secret, rec.Code, rec.Body)
		}
	}
	if typed != 0 {
		t.Fatalf("a stage without the secret reached the terminal %d times", typed)
	}
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/stage", body); rec.Code != http.StatusOK || typed != 1 {
		t.Errorf("type with the secret = %d %s, typed %d", rec.Code, rec.Body, typed)
	}
}
