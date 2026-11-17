package main

import (
	"bytes"
	"encoding/json"
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
	select {
	case <-h.release:
	case <-r.Context().Done():
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(h.replies[req.ID]))
}

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

func takeLate(t *testing.T, in *inbox) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	in.takeLateOpen(rec, httptest.NewRequest(http.MethodPost, "/api/open/late", nil))
	return rec.Code, strings.TrimSpace(rec.Body.String())
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

func TestInbox_KeepsAFailureThatArrivesAfterThePendingReplyAndServesItOnce(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": blocked}}
	in, changed := lateInbox(t, fake)
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	openPending(t, in, "a")
	if status, _ := takeLate(t, in); status != http.StatusNoContent {
		t.Fatalf("a failure was served before the open answered: %d", status)
	}
	close(fake.release)
	waitSignal(t, changed, "the page's call to read again")

	if status, body := takeLate(t, in); status != http.StatusOK || body != `{"message":"macOS blocked opening Terminal.","command":"claude --resume a"}` {
		t.Errorf("the late failure = %d %s", status, body)
	}
	if status, body := takeLate(t, in); status != http.StatusNoContent {
		t.Errorf("the late failure was served twice: %d %s", status, body)
	}
	if text := logged.String(); !strings.Contains(text, "an open answered after the page stopped waiting: not opened") {
		t.Errorf("log = %q; want the late outcome", text)
	}
}

func TestInbox_AnOpenThatSucceedsLateShowsNothing(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{}), replies: map[string]string{"a": `{"opened":true,"message":"Resumed in a new Terminal window."}`}}
	in, _ := lateInbox(t, fake)

	openPending(t, in, "a")
	<-in.refresh
	close(fake.release)
	waitSignal(t, in.refresh, "a list read after the open")

	if status, body := takeLate(t, in); status != http.StatusNoContent {
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
		if status, body := takeLate(t, in); status != http.StatusNoContent {
			t.Fatalf("the older open's failure showed after a newer open: %d %s", status, body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestInbox_AServerThatNeverAnswersAnOpenSaysSoOnceTheReplyDeadlinePasses(t *testing.T) {
	fake := &heldOpens{release: make(chan struct{})}
	t.Cleanup(func() { close(fake.release) })
	in, changed := lateInbox(t, fake)
	in.server.openLateWait = 400 * time.Millisecond
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	openPending(t, in, "a")
	waitSignal(t, changed, "the page's call to read again")

	if status, body := takeLate(t, in); status != http.StatusOK || body != `{"message":"agnostic-ai took too long to answer. Try again."}` {
		t.Errorf("an open never answered = %d %s", status, body)
	}
	host := strings.TrimPrefix(in.server.base, "http://")
	if text := logged.String(); strings.Contains(text, host) || strings.Contains(text, "127.0.0.1") {
		t.Errorf("log = %q; it names the address", text)
	}
}
