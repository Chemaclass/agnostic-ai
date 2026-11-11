package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
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
	server.base, server.version = srv.URL, "1.2.3"
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
	if status, body := getInbox(t, in, ""); status != http.StatusOK || body != `{"view":{"needsYou":2}}` {
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
	if status, body := getInbox(t, in, ""); status != http.StatusOK || body != `{"view":{"needsYou":2}}` {
		t.Errorf("list once the server answers again = %d %s", status, body)
	}
}

func TestInbox_ReadsANewListAfterItStopped(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	in.read(context.Background())

	in.stop()

	if in.latest != nil || in.tag != "" {
		t.Fatalf("a stopped inbox still holds a list (tag %q)", in.tag)
	}
	if status, _ := getInbox(t, in, ""); status != http.StatusOK || fake.reads != 2 {
		t.Errorf("first list after a stop = %d, the server read %d times; want a second read", status, fake.reads)
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

func TestInbox_PassesAMarkToTheServerAndItsAnswerBack(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		in.pass(serverMarkPath)(rec, httptest.NewRequest(http.MethodPost, "/api/mark", strings.NewReader(body)))
		return rec
	}

	done := post(`{"tool":"claude","id":"a","as":"completed"}`)
	gone := post(`{"tool":"claude","id":"gone","as":"completed"}`)

	if done.Code != http.StatusNoContent || done.Body.Len() != 0 {
		t.Errorf("a mark the server kept = %d %s", done.Code, done.Body)
	}
	if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
		t.Errorf("a mark the server refused = %d %s", gone.Code, gone.Body)
	}
	if sent := fake.posts[serverMarkPath]; len(sent) != 2 || sent[0] != `application/json {"tool":"claude","id":"a","as":"completed"}` {
		t.Errorf("the server received %q", sent)
	}
	select {
	case <-in.refresh:
	default:
		t.Error("a mark the server kept asked for no new list")
	}
}

func TestInbox_PassesARenameToTheServerAndItsAnswerBack(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	routes := (&shell{}).routes(in, fstest.MapFS{})
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(body)))
		return rec
	}

	done := post(`{"tool":"claude","id":"a","title":"Parser fix"}`)
	gone := post(`{"tool":"claude","id":"gone","title":"Parser fix"}`)

	if done.Code != http.StatusNoContent || done.Body.Len() != 0 {
		t.Errorf("a rename the server kept = %d %s", done.Code, done.Body)
	}
	if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
		t.Errorf("a rename the server refused = %d %s", gone.Code, gone.Body)
	}
	if sent := fake.posts[serverRenamePath]; len(sent) != 2 || sent[0] != `application/json {"tool":"claude","id":"a","title":"Parser fix"}` {
		t.Errorf("the server received %q", sent)
	}
	select {
	case <-in.refresh:
	default:
		t.Error("a rename the server kept asked for no new list")
	}
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
	if strings.TrimSpace(string(args)) != "serve --global --api-only --no-installed --addr 127.0.0.1:0 --stop-with-stdin --cloud-sessions" {
		t.Errorf("started with %q", args)
	}

	server.rest()

	waitFor(t, "the server to stop once its input closed", func() bool {
		_, err := os.Stat(stopped)
		return err == nil
	})
	if _, err := server.address(); err == nil || err.Error() != "the companion is paused and asks agnostic-ai nothing" {
		t.Errorf("a resting companion started a server: %v", err)
	}
	server.resume()
	if address, err := server.address(); err != nil || address == "" {
		t.Errorf("after resume: %q, %v", address, err)
	}
	server.rest()
}

func TestServer_SaysWhyAgnosticAIDidNotStart(t *testing.T) {
	fakeCommand(t, `echo "Error: unknown flag: --stop-with-stdin" >&2
exit 1`)

	_, err := newServer().address()

	if err == nil || err.Error() != "agnostic-ai serve stopped before it listened: Error: unknown flag: --stop-with-stdin" {
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
		refresh bool
	}{
		"deleted": {fakeAnswer{http.StatusOK, `{"deleted":true,"freedBytes":12000000}`}, true},
		"refused": {fakeAnswer{http.StatusConflict, `{"error":"This conversation is still running. Close it in Claude Code, then delete it.","reason":"running"}`}, false},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverDeletePath: tc.answer}}
			in, _ := connected(t, fake)
			routes := (&shell{}).routes(in, fstest.MapFS{})
			rec := httptest.NewRecorder()

			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/delete", strings.NewReader(`{"tool":"claude","id":"a"}`)))

			if rec.Code != tc.answer.status || rec.Body.String() != tc.answer.body {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
			if sent := fake.posts[serverDeletePath]; len(sent) != 1 || sent[0] != `application/json {"tool":"claude","id":"a"}` {
				t.Errorf("the server received %q", sent)
			}
			asked := false
			select {
			case <-in.refresh:
				asked = true
			default:
			}
			if asked != tc.refresh {
				t.Errorf("asked for a new list: %t", asked)
			}
		})
	}
}
