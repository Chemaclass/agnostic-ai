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

// hold writes a request the hook holds for the fixture's session,
// created at created, and makes its hook process run.
func (f *approvalsFixture) hold(t *testing.T, call string, input approvals.PendingInput, created time.Time) approvals.Pending {
	t.Helper()
	id, err := approvals.NewPendingID()
	if err != nil {
		t.Fatal(err)
	}
	p := approvals.Pending{
		ID: id, Tool: "claude", Session: inProject, Call: call, Input: input,
		Digest:  approvals.PendingDigest("claude", inProject, call, []byte(`{"n":"`+id+`"}`)),
		Created: created, Expires: created.Add(300 * time.Second), PID: 777,
	}
	if err := approvals.WritePending(approvals.PendingDir(f.home), p); err != nil {
		t.Fatal(err)
	}
	if f.h.approvals.alive == nil {
		f.h.approvals.alive = func(pid int) bool { return pid == 777 }
	}
	return p
}

// heldFor is a request that matches the call the fixture's transcript
// shows waiting, read from the row itself.
func (f *approvalsFixture) heldFor(t *testing.T, created time.Time) approvals.Pending {
	t.Helper()
	_, c := f.list(t)
	if c.Waiting == nil {
		t.Fatal("the fixture's row does not wait")
	}
	input := approvals.PendingInput{Command: "go test ./..."}
	call := c.Waiting.Tool
	if call == "" {
		call = "Bash"
	}
	if call != "Bash" {
		input = approvals.PendingInput{FilePath: "/p/" + c.Waiting.Detail}
	} else if c.Waiting.Detail != "" {
		input.Command = c.Waiting.Detail
	}
	return f.hold(t, call, input, created)
}

func pendingOf(c conversations.Conversation) []conversations.PendingRequest {
	if c.Approvals == nil {
		return nil
	}
	return c.Approvals.Pending
}

func decisionOf(t *testing.T, f *approvalsFixture, id string) (approvals.PendingDecision, bool) {
	t.Helper()
	return approvals.ReadDecision(approvals.PendingDir(f.home), id)
}

func TestApprovals_HeldRequestShowsOnItsRow(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	held := f.heldFor(t, f.now.Add(-time.Second))

	view, c := f.list(t)

	got := pendingOf(c)
	if len(got) != 1 || got[0].ID != held.ID || got[0].Digest != held.Digest || got[0].Input != held.Input || got[0].Expires != held.Expires.UTC().Format(time.RFC3339) {
		t.Fatalf("pending = %+v, want the held request %+v", got, held)
	}
	if !needsPermission(c) || view.NeedsYou != 1 {
		t.Errorf("state %s, needsYou %d; a held request needs the person", c.State, view.NeedsYou)
	}
}

func TestApprovals_HeldRequestSkipsTheHookHold(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-200 * time.Millisecond)
	f.wait(t, since)
	f.decide(t, inProject, approvals.Asked, approvals.RulePublish, since)
	f.heldFor(t, since)

	view, c := f.list(t)

	if len(pendingOf(c)) != 1 || !needsPermission(c) || view.NeedsYou != 1 {
		t.Errorf("pending %+v, state %s, needsYou %d", pendingOf(c), c.State, view.NeedsYou)
	}
}

// callWaits ends the fixture's transcript with a Bash call that has no
// result yet, the call Claude Code asks about.
func (f *approvalsFixture) callWaits(t *testing.T, command string) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(f.h.scanner.Stores.ClaudeDir, "projects", "*", inProject+".jsonl"))
	if len(paths) != 1 {
		t.Fatalf("transcripts = %v", paths)
	}
	line, _ := json.Marshal(map[string]any{
		"type": "assistant", "timestamp": "2026-11-07T08:00:01.000Z",
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Bash", "input": map[string]any{"command": command}}}},
	})
	file, err := os.OpenFile(paths[0], os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func TestApprovals_HeldRequestForAnotherCallIsNotShown(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.callWaits(t, "make build")
	f.wait(t, f.now.Add(-time.Second))
	other := f.hold(t, "Bash", approvals.PendingInput{Command: "make other"}, f.now.Add(-10*time.Second))
	read := f.hold(t, "Read", approvals.PendingInput{FilePath: "/p/make build"}, f.now.Add(-9*time.Second))
	same := f.hold(t, "Bash", approvals.PendingInput{Command: "make build"}, f.now.Add(-8*time.Second))

	_, c := f.list(t)

	if c.Waiting == nil || c.Waiting.Tool != "Bash" || c.Waiting.Detail != "make build" {
		t.Fatalf("waiting = %+v", c.Waiting)
	}
	if got := pendingOf(c); len(got) != 1 || got[0].ID != same.ID {
		t.Errorf("pending = %+v; only the request for the waiting call gets buttons", got)
	}
	for _, id := range []string{other.ID, read.ID} {
		if _, released := decisionOf(t, f, id); released {
			t.Error("a request the session may still wait on was released")
		}
	}
}

func TestApprovals_HeldRequestIsWithdrawnWhenTheSessionStopsWaiting(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.busy(t)
	fresh := f.hold(t, "Bash", approvals.PendingInput{Command: "touch a.txt"}, f.now.Add(-time.Second))
	answered := f.hold(t, "Bash", approvals.PendingInput{Command: "touch b.txt"}, f.now.Add(-pendingGrace-time.Second))

	_, c := f.list(t)

	if got := pendingOf(c); len(got) != 0 {
		t.Errorf("pending = %+v on a row that does not wait", got)
	}
	if d, ok := decisionOf(t, f, answered.ID); !ok || d.Decision != approvals.DecideRelease || d.Digest != answered.Digest {
		t.Errorf("answered in the terminal: decision %+v, %v; want release", d, ok)
	}
	if _, ok := decisionOf(t, f, fresh.ID); ok {
		t.Error("a request younger than the grace was released before Claude Code recorded its wait")
	}
}

func TestApprovals_ExpiredOrOrphanedRequestIsRemoved(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	expired := f.hold(t, "Bash", approvals.PendingInput{Command: "touch a.txt"}, f.now.Add(-301*time.Second))
	orphan := f.hold(t, "Bash", approvals.PendingInput{Command: "touch b.txt"}, f.now.Add(-time.Second))
	f.h.approvals.alive = func(pid int) bool { return false }

	_, c := f.list(t)

	if got := pendingOf(c); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
	for _, id := range []string{expired.ID, orphan.ID} {
		if _, ok, _ := approvals.ReadPending(approvals.PendingDir(f.home), id); ok {
			t.Errorf("record %s was kept", id)
		}
	}
}

func TestApprovals_HeldRequestChangesTheListTag(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	tag := listTagOf(t, f.h, "")

	time.Sleep(10 * time.Millisecond)
	f.hold(t, "Bash", approvals.PendingInput{Command: "touch a.txt"}, f.now)

	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("after a held request the list = %d; the buttons must show within one refresh", rec.Code)
	}
}

func TestApprovals_CompanionLinesAreNotApprovedForYou(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	log := approvals.Log{Path: approvals.LogPath(f.home)}
	for _, outcome := range []approvals.Outcome{approvals.Approved, approvals.Approved, approvals.Denied} {
		if err := log.Append(approvals.Entry{Time: f.now.Add(-time.Minute), Tool: "claude", Session: inProject, Call: "Bash", Mode: config.ApprovalsAsk, Decision: outcome, By: approvals.ByCompanion}); err != nil {
			t.Fatal(err)
		}
	}
	f.decide(t, inProject, approvals.Approved, "", f.now.Add(-30*time.Second))

	_, c := f.list(t)

	if got := approvedOf(c); got != 1 {
		t.Errorf("approved for you = %d, want 1: clicks in the companion are the person's own answers", got)
	}
}

func TestApprovalsHandler_DecideWritesOnlyAMatchingDecision(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	reader := newApprovalsReader()
	reader.source = func() (string, error) { return home, nil }
	h := approvalsHandler{reader: reader}
	dir := approvals.PendingDir(home)
	id, _ := approvals.NewPendingID()
	digest := approvals.PendingDigest("claude", "s", "Bash", []byte(`{"command":"touch a.txt"}`))
	if err := approvals.WritePending(dir, approvals.Pending{ID: id, Tool: "claude", Session: "s", Call: "Bash", Digest: digest, Created: now, Expires: now.Add(time.Minute), PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	orphanID, _ := approvals.NewPendingID()
	if err := approvals.WritePending(dir, approvals.Pending{ID: orphanID, Digest: digest, Created: now, Expires: now.Add(time.Minute), PID: 1 << 30}); err != nil {
		t.Fatal(err)
	}
	expiredID, _ := approvals.NewPendingID()
	if err := approvals.WritePending(dir, approvals.Pending{ID: expiredID, Digest: digest, Created: now.Add(-time.Hour), Expires: now.Add(-time.Minute), PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	body := func(id, digest, decision string) string {
		return `{"id":"` + id + `","digest":"` + digest + `","decision":"` + decision + `"}`
	}
	unknown, _ := approvals.NewPendingID()

	for _, tc := range []struct {
		name   string
		body   string
		mutate func(*http.Request)
		want   int
	}{
		{"unknown decision", body(id, digest, "always"), nil, http.StatusBadRequest},
		{"not JSON", "approve", nil, http.StatusBadRequest},
		{"extra field", `{"id":"` + id + `","digest":"` + digest + `","decision":"approve","mode":"yolo"}`, nil, http.StatusBadRequest},
		{"unknown id", body(unknown, digest, "approve"), nil, http.StatusNotFound},
		{"path in id", body("../decisions", digest, "approve"), nil, http.StatusNotFound},
		{"expired", body(expiredID, digest, "approve"), nil, http.StatusNotFound},
		{"hook gone", body(orphanID, digest, "approve"), nil, http.StatusNotFound},
		{"other digest", body(id, strings.Repeat("0", 64), "approve"), nil, http.StatusConflict},
		{"no JSON type", body(id, digest, "approve"), func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"other origin", body(id, digest, "approve"), func(r *http.Request) { r.Header.Set("Origin", "https://example.com") }, http.StatusForbidden},
		{"remote host", body(id, digest, "approve"), func(r *http.Request) { r.Host = "example.com" }, http.StatusMisdirectedRequest},
	} {
		if rec := postEdit(t, h, approvalsDecidePath, tc.body, tc.mutate); rec.Code != tc.want {
			t.Errorf("%s: POST = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+approvalsDecidePath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	if _, ok := approvals.ReadDecision(dir, id); ok {
		t.Fatal("a refused request wrote a decision")
	}

	if rec := postEdit(t, h, approvalsDecidePath, body(id, digest, "deny"), nil); rec.Code != http.StatusOK {
		t.Fatalf("POST deny = %d %s", rec.Code, rec.Body)
	}
	if d, ok := approvals.ReadDecision(dir, id); !ok || d.Decision != approvals.DecideDeny || d.Digest != digest {
		t.Errorf("decision = %+v, %v", d, ok)
	}
	if rec := postEdit(t, h, approvalsDecidePath, body(id, digest, "approve"), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a second answer = %d, want 404", rec.Code)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".decision.json") && e.Name() != id+".decision.json" || strings.HasPrefix(e.Name(), ".") {
			t.Errorf("unexpected decision file %s", filepath.Join(dir, e.Name()))
		}
	}
}

func TestKeepCompanionLive_TouchesUntilStoppedThenRemoves(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, config.ConfigFileName), "approvals:\n  companion: true\n")
	ctx, stop := context.WithCancel(context.Background())
	wait := keepCompanionLive(ctx, home)
	path := approvals.LivePath(home)
	deadline := time.Now().Add(5 * time.Second)
	for !approvals.CompanionLive(path, time.Now()) {
		if time.Now().After(deadline) {
			t.Fatal("companion.live was not written")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stop()
	wait()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("companion.live after stop: %v", err)
	}
}

func TestKeepCompanionLive_WritesNothingWithTheSwitchOff(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, config.ConfigFileName), "approvals:\n  mode: ask\n")
	path := approvals.LivePath(home)
	writeTestFile(t, path, "stale\n")
	ctx, stop := context.WithCancel(context.Background())
	wait := keepCompanionLive(ctx, home)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("companion.live stays with approvals.companion off")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stop()
	wait()
}

func TestApprovals_DecidedRequestOfAGoneHookIsRemoved(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsAsk, true)
	f.wait(t, f.now.Add(-time.Second))
	held := f.hold(t, "Bash", approvals.PendingInput{Command: "touch a.txt"}, f.now.Add(-time.Second))
	if err := approvals.WriteDecision(approvals.PendingDir(f.home), held.ID, approvals.PendingDecision{Decision: approvals.DecideApprove, Digest: held.Digest}); err != nil {
		t.Fatal(err)
	}
	f.h.approvals.alive = func(int) bool { return false }

	f.list(t)

	if entries, _ := os.ReadDir(approvals.PendingDir(f.home)); len(entries) != 0 {
		t.Errorf("pending folder keeps %d files of a hook that is gone", len(entries))
	}
}

// Codex writes nothing while its hook holds a prompt, so the held
// request is the row's wait: it counts as needing the person at once.
func TestApplyApprovals_HeldCodexRequestIsTheRowsWait(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	var view conversations.View
	if err := json.Unmarshal([]byte(`{"needsYou":1,"conversations":[
		{"tool":"codex","id":"t1","live":true,"state":"active","activity":"working"},
		{"tool":"codex","id":"t2","live":true,"state":"unread"},
		{"tool":"codex","id":"t3","live":false,"state":"dormant"},
		{"tool":"codex","id":"t4","live":true,"state":"active","activity":"working"}
	]}`), &view); err != nil {
		t.Fatal(err)
	}
	patch := "*** Begin Patch\n*** Update File: src/app.go\n@@\n-a\n+b\n*** End Patch"
	request := func(id, call string, input approvals.PendingInput) heldRequest {
		return heldRequest{PendingRequest: conversations.PendingRequest{ID: id, Digest: "d" + id, Call: call, Input: input}, created: now.Add(-2 * time.Second)}
	}
	snap := approvalsSnapshot{
		policy: config.ApprovalsPolicy{Home: config.ApprovalsConfig{Mode: config.ApprovalsAsk}}, policyOK: true,
		projectAsk: map[string]string{}, routed: func(string, string) bool { return true },
		pending: map[string][]heldRequest{
			approvalSessionKey("codex", "t1"): {request("a", "Bash", approvals.PendingInput{Command: "touch a.txt"}), request("b", "apply_patch", approvals.PendingInput{Patch: patch})},
			approvalSessionKey("codex", "t2"): {request("c", "apply_patch", approvals.PendingInput{Patch: patch})},
			approvalSessionKey("codex", "t3"): {request("d", "Bash", approvals.PendingInput{Command: "ls"})},
		},
	}

	applyApprovals(&view, snap, "", now, map[string]time.Time{})

	if view.NeedsYou != 2 {
		t.Errorf("needsYou = %d, want 2: t1 newly waits, t2 already counted", view.NeedsYou)
	}
	rows := map[string]conversations.Conversation{}
	for _, c := range view.Conversations {
		rows[c.ID] = c
	}
	if c := rows["t1"]; c.State != conversations.StateBlocked || c.Activity != conversations.ActivityWaiting || len(pendingOf(c)) != 2 || c.Waiting.Detail != "touch a.txt" {
		t.Errorf("t1 = %+v, want blocked on both held requests", c)
	}
	if c := rows["t2"]; c.State != conversations.StateBlocked || len(pendingOf(c)) != 1 || c.Waiting.Detail != "app.go" {
		t.Errorf("t2 = %+v, want blocked on the patch to app.go", c)
	}
	for _, id := range []string{"t3", "t4"} {
		if c := rows[id]; c.State == conversations.StateBlocked || c.Waiting != nil {
			t.Errorf("%s = %+v, want no wait", id, c)
		}
	}
}

// A Codex request no row can show is let go after the grace, so Codex
// shows its own prompt instead of waiting out the hold.
func TestApprovals_CodexRequestWithoutALiveRowIsReleased(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	a := &approvalsReader{source: func() (string, error) { return home, nil }, alive: func(int) bool { return true }}
	dir := approvals.PendingDir(home)
	write := func(session string, created time.Time) string {
		t.Helper()
		id, _ := approvals.NewPendingID()
		if err := approvals.WritePending(dir, approvals.Pending{ID: id, Tool: "codex", Session: session, Call: "Bash", Input: approvals.PendingInput{Command: "ls"}, Digest: "d", Created: created, Expires: now.Add(time.Minute), PID: 1}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	shown := write("live", now.Add(-time.Minute))
	young := write("gone", now.Add(-time.Second))
	orphan := write("gone", now.Add(-time.Minute))
	view := conversations.View{Conversations: []conversations.Conversation{{Tool: conversations.ToolCodex, ID: "live", Live: true}}}

	if got := a.pendingRequests(&view, "", false, now); len(got[approvalSessionKey("codex", "gone")]) != 2 {
		t.Errorf("a filtered list let a request go: %v", got)
	}
	got := a.pendingRequests(&view, "", true, now)

	if len(got[approvalSessionKey("codex", "live")]) != 1 || len(got[approvalSessionKey("codex", "gone")]) != 1 {
		t.Errorf("pending = %v, want the live row's request and the young one", got)
	}
	for id, want := range map[string]bool{shown: false, young: false, orphan: true} {
		d, ok := approvals.ReadDecision(dir, id)
		if ok != want || (ok && d.Decision != approvals.DecideRelease) {
			t.Errorf("%s decision = %+v, %v; want release %v", id, d, ok, want)
		}
	}
}

func TestUnfilteredFirstPage(t *testing.T) {
	for query, want := range map[string]bool{"": true, "editor=cursor": true, "q=fix": false, "tool=codex": false, "before=2026-11-01T00:00:00Z": false} {
		if got := unfilteredFirstPage(query); got != want {
			t.Errorf("unfilteredFirstPage(%q) = %v, want %v", query, got, want)
		}
	}
}
