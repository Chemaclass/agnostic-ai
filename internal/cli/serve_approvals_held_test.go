package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// appendTranscript adds lines to the fixture session's transcript.
func (f *approvalsFixture) appendTranscript(t *testing.T, lines ...map[string]any) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(f.h.scanner.Stores.ClaudeDir, "projects", "*", inProject+".jsonl"))
	if len(paths) != 1 {
		t.Fatalf("transcripts = %v", paths)
	}
	file, err := os.OpenFile(paths[0], os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for _, line := range lines {
		data, _ := json.Marshal(line)
		if _, err := file.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

// bashCall is a transcript line where the agent calls Bash at at.
func bashCall(id, command string, at time.Time) map[string]any {
	return map[string]any{
		"type": "assistant", "timestamp": at.UTC().Format(time.RFC3339Nano),
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": command}}}},
	}
}

// callResult is a transcript line that answers call id.
func callResult(id string, at time.Time) map[string]any {
	return map[string]any{
		"type": "user", "timestamp": at.UTC().Format(time.RFC3339Nano),
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": "ok"}}},
	}
}

func (f *approvalsFixture) postDecision(t *testing.T, p approvals.Pending, decision string, secret string) *httptest.ResponseRecorder {
	t.Helper()
	h := approvalsHandler{reader: f.h.approvals, secret: f.h.companion}
	body := `{"id":"` + p.ID + `","digest":"` + p.Digest + `","decision":"` + decision + `"}`
	return postEdit(t, h, approvalsDecidePath, body, func(r *http.Request) {
		if secret != "" {
			r.Header.Set(companionSecretHeader, secret)
		}
	})
}

func TestApprovals_ListWithoutTheCompanionSecretCarriesNoHeldRequest(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	held := f.heldFor(t, f.now.Add(-time.Second))

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?editor=cursor", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); strings.Contains(body, held.ID) || strings.Contains(body, held.Digest) {
		t.Errorf("a list read without the secret carries the id or digest: %s", body)
	}
	_, c := f.list(t)
	if got := pendingOf(c); len(got) != 1 || got[0].ID != held.ID {
		t.Errorf("with the secret, pending = %+v", got)
	}
	if listTagOf(t, f.h, "") == rec.Header().Get("ETag") {
		t.Error("the lists with and without the secret share a tag")
	}
}

func TestApprovals_DecideNeedsTheCompanionSecret(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	held := f.heldFor(t, f.now.Add(-time.Second))

	if rec := f.postDecision(t, held, approvals.DecideApprove, ""); rec.Code != http.StatusForbidden {
		t.Errorf("without the secret = %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := f.postDecision(t, held, approvals.DecideApprove, strings.Repeat("0", len(testCompanionSecret))); rec.Code != http.StatusForbidden {
		t.Errorf("with another secret = %d, want 403", rec.Code)
	}
	if _, ok := decisionOf(t, f, held.ID); ok {
		t.Fatal("a refused answer wrote a decision")
	}
	f.h.companion = ""
	if rec := f.postDecision(t, held, approvals.DecideApprove, string(testCompanionSecret)); rec.Code != http.StatusNotFound {
		t.Errorf("a server started without a secret = %d, want 404", rec.Code)
	}
	f.h.companion = testCompanionSecret
	if rec := f.postDecision(t, held, approvals.DecideApprove, string(testCompanionSecret)); rec.Code != http.StatusOK {
		t.Fatalf("with the secret = %d %s", rec.Code, rec.Body)
	}
	if d, ok := decisionOf(t, f, held.ID); !ok || d.Decision != approvals.DecideApprove {
		t.Errorf("decision = %+v, %v", d, ok)
	}
}

func TestTakeCompanionSecret_ReadsOnceAndClearsTheEnvironment(t *testing.T) {
	t.Setenv(companionSecretEnv, string(testCompanionSecret))
	if got := takeCompanionSecret(); got != testCompanionSecret {
		t.Errorf("secret = %q", got)
	}
	if _, set := os.LookupEnv(companionSecretEnv); set {
		t.Error("the secret stays in the environment a started process inherits")
	}
	t.Setenv(companionSecretEnv, "short")
	if got := takeCompanionSecret(); got != "" {
		t.Errorf("a short secret = %q, want none", got)
	}
}

// Claude Code leaves a hook running after a terminal answer (decision
// 028). A second prompt for the same command must never offer the first
// request, on the row or to a click made before the next list read.
func TestApprovals_TerminalAnswerThenSecondPromptNeverOffersTheFirst(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.appendTranscript(t, bashCall("toolu_a", "make build", f.now.Add(-31*time.Second)))
	f.wait(t, f.now.Add(-30*time.Second))
	first := f.hold(t, "Bash", approvals.PendingInput{Command: "make build"}, f.now.Add(-30500*time.Millisecond))
	if _, c := f.list(t); len(pendingOf(c)) != 1 || pendingOf(c)[0].ID != first.ID {
		t.Fatalf("the first prompt shows its request, pending = %+v", pendingOf(c))
	}

	// The person answers in the terminal; the agent asks for the same command again.
	f.appendTranscript(t, callResult("toolu_a", f.now.Add(-20*time.Second)), bashCall("toolu_b", "make build", f.now.Add(-2*time.Second)))
	f.wait(t, f.now.Add(-time.Second))

	if rec := f.postDecision(t, first, approvals.DecideApprove, string(testCompanionSecret)); rec.Code != http.StatusNotFound {
		t.Errorf("a click on the first request = %d %s, want 404", rec.Code, rec.Body)
	}
	if d, ok := decisionOf(t, f, first.ID); !ok || d.Decision != approvals.DecideRelease {
		t.Errorf("the first request = %+v, %v; want release, so its hook exits", d, ok)
	}

	second := f.hold(t, "Bash", approvals.PendingInput{Command: "make build"}, f.now.Add(-1500*time.Millisecond))
	_, c := f.list(t)
	if got := pendingOf(c); len(got) != 1 || got[0].ID != second.ID {
		t.Fatalf("pending = %+v, want only the second request", got)
	}
	if rec := f.postDecision(t, second, approvals.DecideApprove, string(testCompanionSecret)); rec.Code != http.StatusOK {
		t.Errorf("a click on the second request = %d %s", rec.Code, rec.Body)
	}
}

func TestApprovals_TwoRequestsForTheWaitingCallShowNoButtons(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.appendTranscript(t, bashCall("toolu_a", "make build", f.now.Add(-3*time.Second)))
	f.wait(t, f.now.Add(-time.Second))
	one := f.hold(t, "Bash", approvals.PendingInput{Command: "make build"}, f.now.Add(-2*time.Second))
	f.hold(t, "Bash", approvals.PendingInput{Command: "make build"}, f.now.Add(-1500*time.Millisecond))

	_, c := f.list(t)

	if got := pendingOf(c); len(got) != 0 {
		t.Errorf("pending = %+v; either request could be the one that waits", got)
	}
	if rec := f.postDecision(t, one, approvals.DecideApprove, string(testCompanionSecret)); rec.Code != http.StatusNotFound {
		t.Errorf("a click on an ambiguous request = %d, want 404", rec.Code)
	}
}

// A poll that gets 304 reads nothing, so the release of a Codex request
// no row shows runs on its own once the grace passes.
func TestApprovals_HiddenCodexRequestIsReleasedAfterTheGraceWithoutAListRead(t *testing.T) {
	home := t.TempDir()
	clock := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	var scheduled []time.Duration
	var checks []func()
	a := &approvalsReader{
		source: func() (string, error) { return home, nil },
		alive:  func(int) bool { return true },
		now:    func() time.Time { return clock },
		rows: func(_ context.Context) (conversations.View, string) {
			return conversations.View{}, ""
		},
		afterFunc: func(d time.Duration, f func()) { scheduled, checks = append(scheduled, d), append(checks, f) },
	}
	dir := approvals.PendingDir(home)
	id, _ := approvals.NewPendingID()
	if err := approvals.WritePending(dir, approvals.Pending{ID: id, Tool: "codex", Session: "hidden", Call: "Bash", Input: approvals.PendingInput{Command: "ls"}, Digest: "d", Created: clock.Add(-time.Second), Expires: clock.Add(time.Minute), PID: 1}); err != nil {
		t.Fatal(err)
	}

	a.pendingRequests(&conversations.View{}, "", true, clock)
	if _, ok := approvals.ReadDecision(dir, id); ok {
		t.Fatal("a request inside the grace was released")
	}
	if len(checks) != 1 || scheduled[0] > pendingGrace {
		t.Fatalf("scheduled checks %v, want one within the grace", scheduled)
	}
	a.pendingRequests(&conversations.View{}, "", true, clock)
	if len(checks) != 1 {
		t.Errorf("a second read scheduled another check for the same deadline: %v", scheduled)
	}

	clock = clock.Add(scheduled[0])
	checks[0]()

	if d, ok := approvals.ReadDecision(dir, id); !ok || d.Decision != approvals.DecideRelease {
		t.Errorf("after the grace, decision = %+v, %v; want release with no list read", d, ok)
	}
}
