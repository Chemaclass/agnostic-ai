package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

// fakeAgnosticAI stands in for the agnostic-ai server.
type fakeAgnosticAI struct {
	// list answers a list request; nil answers 200 with `{"n":<reads>}` and the tag "v1".
	list  func(w http.ResponseWriter, r *http.Request)
	reads int
	// posts are the bodies sent to each path.
	posts map[string][]string
	// answers replies to a post at a path with its status and body; a path without one answers 204.
	answers map[string]fakeAnswer
	// hold keeps every post unanswered until it closes or the caller gives up; nil holds none.
	hold chan struct{}
}

type fakeAnswer struct {
	status int
	body   string
}

func (f *fakeAgnosticAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		if f.posts == nil {
			f.posts = map[string][]string{}
		}
		f.posts[r.URL.Path] = append(f.posts[r.URL.Path], r.Header.Get("Content-Type")+" "+string(body))
		if f.hold != nil {
			select {
			case <-f.hold:
			case <-r.Context().Done():
				return
			}
		}
		if answer, ok := f.answers[r.URL.Path]; ok && !strings.Contains(string(body), "gone") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(answer.status)
			_, _ = w.Write([]byte(answer.body))
			return
		}
		if strings.Contains(string(body), "gone") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"That conversation is not in this list any more. Refresh the page."}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if f.list != nil {
		f.list(w, r)
		return
	}
	if r.Header.Get("If-None-Match") == `"v1"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	f.reads++
	w.Header().Set("ETag", `"v1"`)
	_, _ = w.Write([]byte(`{"needsYou":2}`))
}

// connected is an inbox whose server already runs at a fake agnostic-ai.
func connected(t *testing.T, fake *fakeAgnosticAI) (*inbox, *int) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	server := newServer()
	server.base, server.health.Version = srv.URL, "1.2.3"
	changes := 0
	return newInbox(server, "1.2.3", func() { changes++ }), &changes
}

func getInbox(t *testing.T, in *inbox, query string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	in.list(rec, httptest.NewRequest(http.MethodGet, "/api/inbox"+query, nil))
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestInbox_KeepsTheListWhileTheServerSaysItIsUnchanged(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, changes := connected(t, fake)

	in.read(context.Background())
	in.read(context.Background())
	in.read(context.Background())

	if fake.reads != 1 || *changes != 1 {
		t.Errorf("the server read %d times and the pages were told %d times; want 1 and 1", fake.reads, *changes)
	}
	if status, body := getInbox(t, in, ""); status != http.StatusOK || body != `{"view":{"needsYou":2},"version":"\"v1\""}` {
		t.Errorf("list = %d %s", status, body)
	}
}

func TestInbox_ShowsNoOldListAfterAReadFailed(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, changes := connected(t, fake)
	in.read(context.Background())
	fake.list = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"the store is locked"}`))
	}

	in.read(context.Background())

	if status, body := getInbox(t, in, ""); status != http.StatusBadGateway || body != `{"error":"the store is locked"}` {
		t.Errorf("list after a failed read = %d %s", status, body)
	}
	if *changes != 2 {
		t.Errorf("the pages were told %d times; want 2: the list, then the failure", *changes)
	}

	fake.list = nil
	in.read(context.Background())
	if status, body := getInbox(t, in, ""); status != http.StatusOK || body != `{"view":{"needsYou":2},"version":"\"v1\""}` {
		t.Errorf("list once the server answers again = %d %s", status, body)
	}
}

func TestInbox_SaysWhichListAReadAndTheEventFollow(t *testing.T) {
	fake := &fakeAgnosticAI{list: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte(`{"needsYou":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"matches":1}`))
	}}
	in, _ := connected(t, fake)
	in.read(context.Background())

	_, search := getInbox(t, in, "?q=parser")

	if search != `{"view":{"matches":1},"version":"\"v1\""}` {
		t.Errorf("search = %s; want the version of the list held when it was asked", search)
	}
	if version := in.version(); version != `"v1"` {
		t.Errorf("the event's version = %q", version)
	}
}

func TestInbox_AsksTheServerForASearch(t *testing.T) {
	asked := ""
	fake := &fakeAgnosticAI{list: func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"matches":1}`))
	}}
	in, _ := connected(t, fake)

	status, body := getInbox(t, in, "?q=fix+the+%26+parser&before=2026-11-01T12:00:00Z")

	if status != http.StatusOK || body != `{"view":{"matches":1}}` || asked != "before=2026-11-01T12%3A00%3A00Z&q=fix+the+%26+parser" {
		t.Errorf("search = %d %s, the server was asked %q", status, body, asked)
	}
}

func TestInbox_SaysWhenItWasBuiltForAnotherRelease(t *testing.T) {
	in, _ := connected(t, &fakeAgnosticAI{})
	in.builtFor = "1.2.0"

	_, body := getInbox(t, in, "")

	var reply inboxReply
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Note != "This companion was built for agnostic-ai 1.2.0 and 1.2.3 is installed. Run: agnostic-ai companion install" {
		t.Errorf("note = %q", reply.Note)
	}
}

// listOfPosts answers a list read with the number of changes the server
// took, so a list read after a change differs from one read before it.
func listOfPosts(fake *fakeAgnosticAI) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		posts := 0
		for _, sent := range fake.posts {
			posts += len(sent)
		}
		tag := fmt.Sprintf(`"v%d"`, posts)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", tag)
		_, _ = fmt.Fprintf(w, `{"posts":%d}`, posts)
	}
}

// heldAfter fails unless the list the companion holds is want: the
// pages read the held list as soon as a change answers.
func heldAfter(t *testing.T, in *inbox, change, want string) {
	t.Helper()
	if held := string(in.held()); held != want {
		t.Errorf("the list held once %s answered = %s; want %s", change, held, want)
	}
}

func TestInbox_PassesAMarkToTheServerAndItsAnswerBack(t *testing.T) {
	fake := &fakeAgnosticAI{}
	fake.list = listOfPosts(fake)
	in, changes := connected(t, fake)
	in.read(context.Background())
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark", strings.NewReader(body)))
		return rec
	}

	done := post(`{"tool":"claude","id":"a","as":"completed"}`)
	heldAfter(t, in, "a kept mark", `{"posts":1}`)
	gone := post(`{"tool":"claude","id":"gone","as":"completed"}`)
	heldAfter(t, in, "a refused mark", `{"posts":1}`)

	if done.Code != http.StatusNoContent || done.Body.Len() != 0 || done.Header().Get(listVersionHeader) != `"v1"` {
		t.Errorf("a mark the server kept = %d %s, list version %q", done.Code, done.Body, done.Header().Get(listVersionHeader))
	}
	if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` || gone.Header().Get(listVersionHeader) != "" {
		t.Errorf("a mark the server refused = %d %s, list version %q", gone.Code, gone.Body, gone.Header().Get(listVersionHeader))
	}
	if sent := fake.posts[serverMarkPath]; len(sent) != 2 || sent[0] != `application/json {"tool":"claude","id":"a","as":"completed"}` {
		t.Errorf("the server received %q", sent)
	}
	if *changes != 2 {
		t.Errorf("the pages were told %d times; want 2: the first list, then the marked one", *changes)
	}
}

func TestInbox_AKeptMarkAnswersWhenTheListReadAfterItFails(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	in.read(context.Background())
	fake.list = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"the store is locked"}`))
	}
	rec := httptest.NewRecorder()

	in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark", strings.NewReader(`{"tool":"claude","id":"a","as":"completed"}`)))

	if rec.Code != http.StatusNoContent {
		t.Errorf("a kept mark whose list read failed = %d %s; want 204", rec.Code, rec.Body)
	}
	if status, body := getInbox(t, in, ""); status != http.StatusBadGateway || body != `{"error":"the store is locked"}` {
		t.Errorf("list after the failed read = %d %s", status, body)
	}
}

func TestInbox_AKeptMarkAnswersWithoutWaitingLongForTheList(t *testing.T) {
	held := make(chan struct{})
	t.Cleanup(func() { close(held) })
	fake := &fakeAgnosticAI{list: func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-held:
		case <-r.Context().Done():
		}
	}}
	in, _ := connected(t, fake)
	in.changeWait = 50 * time.Millisecond
	rec := httptest.NewRecorder()
	began := time.Now()

	in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark", strings.NewReader(`{"tool":"claude","id":"a","as":"completed"}`)))

	if took := time.Since(began); rec.Code != http.StatusNoContent || took > 2*time.Second || rec.Header().Get(listVersionHeader) != "" {
		t.Errorf("a kept mark whose list read hangs = %d after %s, list version %q; want 204 at once and no version", rec.Code, took, rec.Header().Get(listVersionHeader))
	}
	select {
	case <-in.refresh:
	default:
		t.Error("the mark left the list to no one: the poll loop was not asked for it")
	}
}

func TestInbox_AMarkInARunLeavesTheListToThePollLoop(t *testing.T) {
	fake := &fakeAgnosticAI{}
	fake.list = listOfPosts(fake)
	in, _ := connected(t, fake)
	in.read(context.Background())
	rec := httptest.NewRecorder()

	in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark?read=later", strings.NewReader(`{"tool":"claude","id":"a","as":"completed"}`)))

	if rec.Code != http.StatusNoContent {
		t.Errorf("a mark in a run = %d %s", rec.Code, rec.Body)
	}
	heldAfter(t, in, "a mark in a run", `{"posts":0}`)
	select {
	case <-in.refresh:
	default:
		t.Error("a mark in a run asked for no new list")
	}
}

func TestInbox_PassesARenameToTheServerAndItsAnswerBack(t *testing.T) {
	fake := &fakeAgnosticAI{}
	fake.list = listOfPosts(fake)
	in, _ := connected(t, fake)
	in.read(context.Background())
	routes := (&shell{}).routes(in, fstest.MapFS{})
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(body)))
		return rec
	}

	done := post(`{"tool":"claude","id":"a","title":"Parser fix"}`)
	heldAfter(t, in, "a kept rename", `{"posts":1}`)
	gone := post(`{"tool":"claude","id":"gone","title":"Parser fix"}`)
	heldAfter(t, in, "a refused rename", `{"posts":1}`)

	if done.Code != http.StatusNoContent || done.Body.Len() != 0 {
		t.Errorf("a rename the server kept = %d %s", done.Code, done.Body)
	}
	if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
		t.Errorf("a rename the server refused = %d %s", gone.Code, gone.Body)
	}
	if sent := fake.posts[serverRenamePath]; len(sent) != 2 || sent[0] != `application/json {"tool":"claude","id":"a","title":"Parser fix"}` {
		t.Errorf("the server received %q", sent)
	}
}

func TestInbox_PassesAskMeToTheServerAndReadsTheListItChanged(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverApprovalsAskPath: {http.StatusOK, `{"mode":"ask","path":"/home/local/agnostic-ai.yaml"}`}}}
	fake.list = listOfPosts(fake)
	in, _ := connected(t, fake)
	in.read(context.Background())
	routes := (&shell{}).routes(in, fstest.MapFS{})
	rec := httptest.NewRecorder()

	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/approvals/ask", strings.NewReader(`{"mode":"ask"}`)))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"ask"`) {
		t.Errorf("Ask me = %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts[serverApprovalsAskPath]; len(sent) != 1 || sent[0] != `application/json {"mode":"ask"}` {
		t.Errorf("the server received %q", sent)
	}
	heldAfter(t, in, "Ask me", `{"posts":1}`)
}

// fakeCommand writes a program that stands in for agnostic-ai and
// points AGNOSTIC_AI_BIN at it.
func fakeCommand(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in command is a shell script")
	}
	path := filepath.Join(t.TempDir(), "agnostic-ai")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_BIN", path)
}

func TestServer_StartsAgnosticAIAndStopsItWhenResting(t *testing.T) {
	stopped := filepath.Join(t.TempDir(), "stopped")
	// Like serve --stop-with-stdin: say the address, then run until the input closes.
	fakeCommand(t, `echo "$@" > "`+stopped+`.args"
echo "Serving agnostic-ai at http://127.0.0.1:1/"
cat > /dev/null
touch "`+stopped+`"`)
	server := newServer()

	address, err := server.address()

	if err != nil || address != "http://127.0.0.1:1" {
		t.Fatalf("address = %q, %v", address, err)
	}
	args, _ := os.ReadFile(stopped + ".args")
	if strings.TrimSpace(string(args)) != "serve --global --api-only --no-installed --addr 127.0.0.1:0 --stop-with-stdin --cloud-sessions --require-companion-secret" {
		t.Errorf("started with %q", args)
	}

	server.rest()

	waitFor(t, "the server to stop once its input closed", func() bool {
		_, err := os.Stat(stopped)
		return err == nil
	})
	if _, err := server.address(); err == nil || err.Error() != "the companion is quitting and asks agnostic-ai nothing" {
		t.Errorf("a resting companion started a server: %v", err)
	}
}

func TestServer_HandsTheServerOnlyItsOwnSecret(t *testing.T) {
	t.Setenv(secretEnv, "a secret this companion does not know")
	seen := filepath.Join(t.TempDir(), "secret")
	fakeCommand(t, `printf '%s' "$`+secretEnv+`" > "`+seen+`"
echo "Serving agnostic-ai at http://127.0.0.1:1/"
cat > /dev/null`)
	server := newServer()
	server.secret = ""

	if _, err := server.address(); err != nil {
		t.Fatal(err)
	}
	server.rest()

	if got, _ := os.ReadFile(seen); len(got) != 0 {
		t.Errorf("the server saw secret %q; want this companion's empty one", got)
	}
}

func TestServer_SaysWhyAgnosticAIDidNotStart(t *testing.T) {
	fakeCommand(t, `echo "Error: the config folder is not readable" >&2
exit 1`)

	_, err := newServer().address()

	if err == nil || err.Error() != "agnostic-ai serve stopped before it listened: Error: the config folder is not readable" {
		t.Errorf("err = %v", err)
	}
}

func TestServer_SaysWhenTheCommandIsMissing(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_BIN", filepath.Join(t.TempDir(), "nowhere"))

	_, err := newServer().address()

	if err == nil || !strings.HasPrefix(err.Error(), "AGNOSTIC_AI_BIN: ") {
		t.Errorf("err = %v", err)
	}
}

func TestInbox_PassesADeleteToTheServerAndItsAnswerBack(t *testing.T) {
	for name, tc := range map[string]struct {
		answer  fakeAnswer
		held    string
		version string
	}{
		"deleted": {fakeAnswer{http.StatusOK, `{"deleted":true,"freedBytes":12000000}`}, `{"posts":1}`, `"v1"`},
		"refused": {fakeAnswer{http.StatusConflict, `{"error":"This conversation is still running. Close it in Claude Code, then delete it.","reason":"running"}`}, `{"posts":0}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverDeletePath: tc.answer}}
			fake.list = listOfPosts(fake)
			in, _ := connected(t, fake)
			in.read(context.Background())
			routes := (&shell{}).routes(in, fstest.MapFS{})
			rec := httptest.NewRecorder()

			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/delete", strings.NewReader(`{"tool":"claude","id":"a"}`)))

			if rec.Code != tc.answer.status || rec.Body.String() != tc.answer.body {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
			if sent := fake.posts[serverDeletePath]; len(sent) != 1 || sent[0] != `application/json {"tool":"claude","id":"a"}` {
				t.Errorf("the server received %q", sent)
			}
			heldAfter(t, in, "the delete", tc.held)
			if version := rec.Header().Get(listVersionHeader); version != tc.version {
				t.Errorf("list version = %q; want %q", version, tc.version)
			}
		})
	}
}

func TestInbox_AnOpenTheServerIsSlowToAnswerIsStillOpeningNotAFailure(t *testing.T) {
	fake := &fakeAgnosticAI{hold: make(chan struct{})}
	in, _ := connected(t, fake)
	in.server.openWait = 30 * time.Millisecond
	rec := httptest.NewRecorder()

	in.pass(serverOpenPath)(rec, httptest.NewRequest(http.MethodPost, "/api/open", strings.NewReader(`{"tool":"claude","id":"a"}`)))

	if body := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusAccepted || body != `{"pending":true,"message":"Still opening. This takes longer than usual."}` {
		t.Errorf("a slow open = %d %s; want 202 and no failure, since the conversation may still open", rec.Code, body)
	}
	// The open goes on after the reply: wait for it to end, so it writes to no later test's log.
	<-in.refresh
	close(fake.hold)
	select {
	case <-in.refresh:
	case <-time.After(10 * time.Second):
		t.Fatal("the open never ended")
	}
}

func TestInbox_ACallTheServerDoesNotAnswerSaysSoInPlainWords(t *testing.T) {
	fake := &fakeAgnosticAI{hold: make(chan struct{})}
	in, _ := connected(t, fake)
	in.server.callWait = 30 * time.Millisecond
	rec := httptest.NewRecorder()

	in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark", strings.NewReader(`{"tool":"claude","id":"a","as":"seen"}`)))

	if body := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusGatewayTimeout || body != `{"error":"agnostic-ai took too long to answer. Try again."}` {
		t.Errorf("a mark the server did not answer in time = %d %s", rec.Code, body)
	}
}

func TestInbox_LogsAServerThatIsGoneWithoutItsAddressOrTheWordsSearched(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	server := newServer()
	server.base = gone.URL
	in := newInbox(server, "", func() {})
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	in.read(context.Background())
	getInbox(t, in, "?q=secret+plans")

	host := strings.TrimPrefix(gone.URL, "http://")
	if text := logged.String(); !strings.Contains(text, "ask agnostic-ai "+serverListPath+": ") || strings.Contains(text, host) || strings.Contains(text, "127.0.0.1") || strings.Contains(text, "secret") {
		t.Errorf("log = %q; want the path and the cause, without the address or the words searched", text)
	}
}

func TestInbox_AServerThatIsGoneSaysSoInPlainWords(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	server := newServer()
	server.base = gone.URL
	in := newInbox(server, "", func() {})

	in.read(context.Background())
	rec := httptest.NewRecorder()
	in.pass(serverOpenPath)(rec, httptest.NewRequest(http.MethodPost, "/api/open", strings.NewReader(`{"tool":"claude","id":"a"}`)))

	const plain = `{"error":"The companion could not reach agnostic-ai. Try again."}`
	if status, body := getInbox(t, in, ""); status != http.StatusBadGateway || body != plain {
		t.Errorf("the list = %d %s; want plain words and no Go error or address", status, body)
	}
	if body := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadGateway || body != plain {
		t.Errorf("an open = %d %s; a real failure still answers as one", rec.Code, body)
	}
}

func TestInbox_PassesStopShellAndRefreshesTheList(t *testing.T) {
	const payload = `{"tool":"claude","id":"a","pid":90,"procStart":"shell-start"}`
	answer := fakeAnswer{http.StatusOK, `{"stillRunning":[{"pid":91,"procStart":"child-start"}]}`}
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{"/api/conversations/stop-shell": answer}}
	fake.list = listOfPosts(fake)
	in, _ := connected(t, fake)
	in.read(context.Background())
	routes := (&shell{}).routes(in, fstest.MapFS{})
	rec := httptest.NewRecorder()

	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/stop-shell", strings.NewReader(payload)))

	if rec.Code != answer.status || rec.Body.String() != answer.body {
		t.Errorf("= %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts["/api/conversations/stop-shell"]; len(sent) != 1 || sent[0] != "application/json "+payload {
		t.Errorf("the server received %q", sent)
	}
	heldAfter(t, in, "the stop", `{"posts":1}`)
	if version := rec.Header().Get(listVersionHeader); version != `"v1"` {
		t.Errorf("list version = %q; want v1", version)
	}
}

func TestInbox_ShowsTheConfigurationFolderReadByTheServer(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom global setup")
	var healthRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			if healthRequests.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(serverHealth{OK: true, Global: true, Root: root})
			return
		}
		_, _ = io.WriteString(w, `{"conversations":[]}`)
	}))
	defer srv.Close()
	fakeCommand(t, "echo 'Serving agnostic-ai at "+srv.URL+"/'\ncat >/dev/null")
	server := newServer()
	defer server.rest()
	in := newInbox(server, "", func() {})
	status, body := getInbox(t, in, "")
	var reply inboxReply
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || reply.ConfigRoot != root {
		t.Errorf("inbox = %d %s, want configuration folder %q", status, body, root)
	}
}

func TestInbox_PassesADecisionFromThePageOnly(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverApprovalsDecidePath: {http.StatusOK, `{"decision":"approve"}`}}}
	fake.list = listOfPosts(fake)
	in, _ := connected(t, fake)
	in.read(context.Background())
	routes := (&shell{}).routes(in, fstest.MapFS{})
	body := `{"id":"0123456789abcdef0123456789abcdef","digest":"d","decision":"approve"}`

	for name, mutate := range map[string]func(*http.Request){
		"another origin": func(r *http.Request) { r.Header.Set("Origin", "https://example.com") },
		"cross site":     func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"plain text":     func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
	} {
		req := httptest.NewRequest(http.MethodPost, serverApprovalsDecidePath, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mutate(req)
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code < 400 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if sent := fake.posts[serverApprovalsDecidePath]; len(sent) != 0 {
		t.Fatalf("a refused request reached the server: %q", sent)
	}

	req := httptest.NewRequest(http.MethodPost, serverApprovalsDecidePath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("decide = %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts[serverApprovalsDecidePath]; len(sent) != 1 || sent[0] != "application/json "+body {
		t.Errorf("the server received %q", sent)
	}
}
