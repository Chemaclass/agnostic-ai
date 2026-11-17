package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// heldOpens answers each open with the reply its id names, once the test releases it.
type heldOpens struct {
	release chan struct{}
	replies map[string]string
	mu      sync.Mutex
	asked   []string
}

func (h *heldOpens) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	_ = json.NewDecoder(r.Body).Decode(&req)
	h.mu.Lock()
	h.asked = append(h.asked, req.ID)
	h.mu.Unlock()
	if strings.HasPrefix(req.ID, "quick") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(h.replies[req.ID]))
		return
	}
	select {
	case <-h.release:
	case <-r.Context().Done():
		return
	}
	reply, ok := h.replies[req.ID]
	switch {
	case reply == dropped:
		// The server's write deadline passed: the connection closes with no answer.
		connection, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	case !ok:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}
}

// dropped is a reply the fake never writes: it closes the connection instead.
const dropped = "dropped"

// lockedLog takes the companion's log while the test runs, for a goroutine that may still write to it.
func lockedLog(t *testing.T) *lockedBuffer {
	t.Helper()
	logged := &lockedBuffer{}
	log.SetOutput(logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return logged
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var _ io.Writer = (*lockedBuffer)(nil)

// lateInbox is an inbox whose opens go to fake and stop being waited for after a moment.
func lateInbox(t *testing.T, fake http.Handler) (*inbox, chan struct{}) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	server := newServer()
	server.base, server.version = srv.URL, "1.2.3"
	server.openWait, server.openLateWait = 50*time.Millisecond, 10*time.Second
	changed := make(chan struct{}, 8)
	return newInbox(server, "1.2.3", func() { changed <- struct{}{} }), changed
}

func openPending(t *testing.T, in *inbox, id string) {
	t.Helper()
	rec := httptest.NewRecorder()
	in.pass(serverOpenPath)(rec, httptest.NewRequest(http.MethodPost, "/api/open", strings.NewReader(`{"tool":"claude","id":"`+id+`"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the open of %s = %d %s; this test needs one still opening", id, rec.Code, rec.Body)
	}
}

func readLate(t *testing.T, in *inbox) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	in.lateOpen(rec, httptest.NewRequest(http.MethodGet, "/api/open/late", nil))
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func dismissLate(t *testing.T, in *inbox, id string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	in.dismissLateOpen(rec, httptest.NewRequest(http.MethodPost, "/api/open/late/dismiss", strings.NewReader(`{"id":`+id+`}`)))
	return rec.Code
}

func waitSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s never came", what)
	}
}

const blocked = `{"opened":false,"message":"macOS blocked opening Terminal.","command":"claude --resume a"}`

// heldList is the list the companion holds, with the chat "a" under title.
func heldList(in *inbox, title string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.latest = json.RawMessage(`{"conversations":[{"tool":"claude","id":"a","title":"` + title + `"},{"tool":"codex","id":"a","title":"Another tool"}]}`)
}

func TestInbox_KeepsALateFailureWithItsChatUntilItIsDismissed(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": blocked}}
	in, changed := lateInbox(t, fake)
	logged := lockedLog(t)
	heldList(in, "Tidy the old branches")

	openPending(t, in, "a")
	if status, _ := readLate(t, in); status != http.StatusNoContent {
		t.Fatalf("a failure was served before the open answered: %d", status)
	}
	close(fake.release)
	waitSignal(t, changed, "the page's call to read again")

	const kept = `{"id":1,"title":"Tidy the old branches","message":"macOS blocked opening Terminal.","command":"claude --resume a"}`
	for range 2 {
		if status, body := readLate(t, in); status != http.StatusOK || body != kept {
			t.Errorf("the late failure = %d %s; want %s on every read", status, body, kept)
		}
	}
	if status := dismissLate(t, in, "2"); status != http.StatusNoContent {
		t.Errorf("dismissing another failure = %d", status)
	}
	if status, body := readLate(t, in); body != kept {
		t.Errorf("dismissing another failure removed this one: %d %s", status, body)
	}
	if status := dismissLate(t, in, "1"); status != http.StatusNoContent {
		t.Errorf("the dismiss = %d", status)
	}
	if status, body := readLate(t, in); status != http.StatusNoContent {
		t.Errorf("a dismissed failure is still served: %d %s", status, body)
	}
	if text := logged.String(); !strings.Contains(text, "an open the page stopped waiting for: not opened") {
		t.Errorf("log = %q; want the late outcome", text)
	}
}

func TestInbox_ALateFailureNamesTheChatAsTheListNamesItNow(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": blocked}}
	in, changed := lateInbox(t, fake)
	lockedLog(t)
	heldList(in, "Tidy the old branches")

	openPending(t, in, "a")
	close(fake.release)
	waitSignal(t, changed, "the page's call to read again")
	heldList(in, "Tidy the branches")

	if _, body := readLate(t, in); !strings.Contains(body, `"title":"Tidy the branches"`) {
		t.Errorf("the late failure of a renamed chat = %s", body)
	}
	in.mu.Lock()
	in.latest = json.RawMessage(`{"conversations":[]}`)
	in.mu.Unlock()
	if status, body := readLate(t, in); status != http.StatusOK || strings.Contains(body, "title") {
		t.Errorf("a chat the list no longer holds = %d %s; want the failure without a title", status, body)
	}
}

func TestInbox_AnOpenThatSucceedsLateShowsNothing(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": `{"opened":true,"message":"Resumed in a new Terminal window."}`}}
	in, _ := lateInbox(t, fake)

	openPending(t, in, "a")
	<-in.refresh
	close(fake.release)
	waitSignal(t, in.refresh, "a list read after the open")

	if status, body := readLate(t, in); status != http.StatusNoContent {
		t.Errorf("a late success = %d %s; want nothing to show", status, body)
	}
}

func TestInbox_ANewerOpenWinsOverAnOlderLateFailure(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": blocked, "b": `{"opened":true,"message":"Opened."}`}}
	in, _ := lateInbox(t, fake)

	openPending(t, in, "a")
	openPending(t, in, "b")
	<-in.refresh
	close(fake.release)
	// Both late answers settle, each asking for a list read.
	waitSignal(t, in.refresh, "a list read after the opens")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if status, body := readLate(t, in); status != http.StatusNoContent {
			t.Fatalf("the older open's failure showed after a newer open: %d %s", status, body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// noAnswer is what the page reads when an open it stopped waiting for got no answer at all.
const noAnswer = `{"id":1,"message":"agnostic-ai did not answer in time. The chat may still have opened."}`

func TestInbox_AnOpenThatGetsNoAnswerAfterThePendingReplySaysItMayStillHaveOpened(t *testing.T) {
	for name, tc := range map[string]struct {
		reply    string
		lateWait time.Duration
	}{
		"the server drops the connection": {reply: dropped, lateWait: 10 * time.Second},
		"the server never answers":        {lateWait: 400 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{}}
			if tc.reply != "" {
				fake.replies["a"] = tc.reply
			}
			in, changed := lateInbox(t, fake)
			in.server.openLateWait = tc.lateWait
			logged := lockedLog(t)

			openPending(t, in, "a")
			if tc.reply != "" {
				time.Sleep(300 * time.Millisecond)
				close(fake.release)
			} else {
				t.Cleanup(func() { close(fake.release) })
			}
			waitSignal(t, changed, "the page's call to read again")

			if status, body := readLate(t, in); status != http.StatusOK || body != noAnswer {
				t.Errorf("an open with no answer = %d %s; want %s", status, body, noAnswer)
			}
			host := strings.TrimPrefix(in.server.base, "http://")
			if text := logged.String(); strings.Contains(text, host) || strings.Contains(text, "127.0.0.1") || !strings.Contains(text, "an open the page stopped waiting for: no answer") {
				t.Errorf("log = %q; want the outcome without the address", text)
			}
		})
	}
}

func TestInbox_ALateAnswerThatDoesNotSayItOpenedIsNotLoggedAsOpened(t *testing.T) {
	for name, reply := range map[string]string{"no content": "", "a body that is no reply": "not json"} {
		t.Run(name, func(t *testing.T) {
			fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{}}
			if reply != "" {
				fake.replies["a"] = reply
			}
			in, _ := lateInbox(t, fake)
			logged := lockedLog(t)

			openPending(t, in, "a")
			<-in.refresh
			close(fake.release)
			waitSignal(t, in.refresh, "a list read after the open")

			if text := logged.String(); strings.Contains(text, ": opened") || !strings.Contains(text, "an open the page stopped waiting for: no word on whether it opened") {
				t.Errorf("log = %q", text)
			}
			if status, body := readLate(t, in); status != http.StatusNoContent {
				t.Errorf("an answer that says nothing = %d %s; want nothing to show", status, body)
			}
		})
	}
}

func TestInbox_OpeningTheSameChatAgainWithSuccessDropsItsLateFailure(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": blocked, "quick-a": `{"opened":true,"message":"Opened."}`}}
	in, changed := lateInbox(t, fake)
	lockedLog(t)

	openPending(t, in, "a")
	close(fake.release)
	waitSignal(t, changed, "the page's call to read again")
	rec := httptest.NewRecorder()
	in.pass(serverOpenPath)(rec, httptest.NewRequest(http.MethodPost, "/api/open", strings.NewReader(`{"tool":"claude","id":"quick-a"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("the second open = %d %s", rec.Code, rec.Body)
	}

	if status, body := readLate(t, in); status != http.StatusNoContent {
		t.Errorf("the failure stayed after the chat opened: %d %s", status, body)
	}
}
