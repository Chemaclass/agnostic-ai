package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

func TestMailboxProxy_ForwardsTheThreadReadWithItsQuery(t *testing.T) {
	const threads = `{"threads":[]}`
	fake := &fakeAgnosticAI{
		list: func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/api/mailbox/threads" ||
				r.URL.Query().Get("tool") != "claude" || r.URL.Query().Get("id") != "some-id" {
				t.Errorf("forwarded thread read = %s %s", r.Method, r.URL)
			}
			_, _ = w.Write([]byte(threads))
		},
	}
	in, _ := connected(t, fake)
	rec := httptest.NewRecorder()
	(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/mailbox/threads?tool=claude&id=some-id", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != threads {
		t.Errorf("thread read = %d %s", rec.Code, rec.Body)
	}
}

func TestMailboxProxy_PreservesRefusalsAndRefreshesOnlyKeptChanges(t *testing.T) {
	const refusal = `{"guard":"paused","error":"Thread is paused."}`
	const stage = `{"staged":false,"text":"Use this data."}`
	fake := &fakeAgnosticAI{
		answers: map[string]fakeAnswer{
			"/api/mailbox/stage": {status: http.StatusOK, body: stage},
			"/api/mailbox/send":  {status: http.StatusUnprocessableEntity, body: refusal},
		},
	}
	in, _ := connected(t, fake)
	routes := (&shell{}).routes(in, fstest.MapFS{})
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routes.ServeHTTP(rec, req)
		return rec
	}
	prepared := post("/api/mailbox/stage", `{"thread":"some-id","n":1,"how":"copy"}`)
	if prepared.Code != http.StatusOK || prepared.Body.String() != stage || prepared.Header().Get(listVersionHeader) != "" {
		t.Errorf("prepared copy = %d %s, version %q", prepared.Code, prepared.Body, prepared.Header().Get(listVersionHeader))
	}
	denied := post("/api/mailbox/send", `{"type":"question"}`)
	if denied.Code != http.StatusUnprocessableEntity || denied.Body.String() != refusal || denied.Header().Get(listVersionHeader) != "" {
		t.Errorf("refused send = %d %s, version %q", denied.Code, denied.Body, denied.Header().Get(listVersionHeader))
	}
	changed := post("/api/mailbox/thread", `{"thread":"some-id","action":"resume"}`)
	if changed.Code != http.StatusNoContent || changed.Header().Get(listVersionHeader) == "" {
		t.Errorf("thread change = %d %s, version %q", changed.Code, changed.Body, changed.Header().Get(listVersionHeader))
	}
}

func TestMailboxProxy_RejectsForeignPagesAndWrongContentTypes(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	routes := (&shell{}).routes(in, fstest.MapFS{})
	for _, test := range []struct {
		name, method, path, origin, contentType string
		want                                    int
	}{
		{"foreign read", http.MethodGet, "/api/mailbox/threads", "https://foreign.example", "", http.StatusForbidden},
		{"foreign write", http.MethodPost, "/api/mailbox/send", "https://foreign.example", "application/json", http.StatusForbidden},
		{"wrong content type", http.MethodPost, "/api/mailbox/send", "wails://localhost", "text/plain", http.StatusUnsupportedMediaType},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "wails://localhost"+test.path, strings.NewReader("{}"))
			req.Header.Set("Origin", test.origin)
			req.Header.Set("Content-Type", test.contentType)
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)
			if rec.Code != test.want {
				t.Errorf("mailbox proxy = %d %s, want %d", rec.Code, rec.Body, test.want)
			}
		})
	}
	if fake.reads != 0 || len(fake.posts) != 0 {
		t.Errorf("the companion forwarded refused requests: %d reads, %v writes", fake.reads, fake.posts)
	}
}

func TestMailboxProxy_CopiesNativelyBeforeConfirming(t *testing.T) {
	const text = "<agnostic-ai-mailbox>\nA prepared message.\n</agnostic-ai-mailbox>"
	for _, test := range []struct {
		name        string
		writeOK     bool
		confirmCode int
		wantStaged  bool
		wantCopied  bool
		wantMessage string
	}{
		{"success", true, http.StatusOK, true, false, "Message copied to the clipboard."},
		{"clipboard refused", false, http.StatusOK, false, false, "still waiting"},
		{"confirm refused", true, http.StatusConflict, false, true, "could not mark it staged"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var writes, prepares, confirms, listReads atomic.Int32
			var clipboardOK atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case serverMailboxStagePath:
					var req struct {
						Thread  string `json:"thread"`
						Number  int    `json:"n"`
						How     string `json:"how"`
						Confirm bool   `json:"confirm"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("server received invalid stage JSON: %v", err)
					}
					if r.Method != http.MethodPost || req.Thread != "some-id" || req.Number != 1 || req.How != "copy" {
						t.Errorf("server received %s stage %+v", r.Method, req)
					}
					if !req.Confirm {
						prepares.Add(1)
						writeJSON(w, http.StatusOK, map[string]any{"staged": false, "text": text})
						return
					}
					confirms.Add(1)
					if !clipboardOK.Load() {
						t.Error("confirmation reached the server before the native clipboard accepted the text")
					}
					if test.confirmCode != http.StatusOK {
						writeJSON(w, test.confirmCode, apiError{Error: "The mailbox is busy."})
						return
					}
					writeJSON(w, http.StatusOK, map[string]any{"staged": true, "message": "Message copied to the clipboard."})
				case serverListPath:
					listReads.Add(1)
					w.Header().Set("ETag", `"after-copy"`)
					writeJSON(w, http.StatusOK, map[string]any{"needsYou": 0})
				default:
					t.Errorf("unexpected upstream path %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			server := newServer()
			server.base, server.health.Version = upstream.URL, "1.2.3"
			in := newInbox(server, "1.2.3", func() {})
			in.writeClipboard = func(_ context.Context, got string) bool {
				writes.Add(1)
				if got != text {
					t.Errorf("native clipboard got %q, want %q", got, text)
				}
				clipboardOK.Store(test.writeOK)
				return test.writeOK
			}

			req := httptest.NewRequest(http.MethodPost, serverMailboxStagePath, strings.NewReader(`{"thread":"some-id","n":1,"how":"copy"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, req)
			var result struct {
				Staged, Copied bool
				Message, Text  string
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || result.Staged != test.wantStaged || result.Copied != test.wantCopied ||
				!strings.Contains(result.Message, test.wantMessage) || result.Text != "" {
				t.Errorf("native copy = %d %s", rec.Code, rec.Body)
			}
			if prepares.Load() != 1 || writes.Load() != 1 || (confirms.Load() == 1) != test.writeOK {
				t.Errorf("prepare/write/confirm calls = %d/%d/%d", prepares.Load(), writes.Load(), confirms.Load())
			}
			if (listReads.Load() == 1) != test.wantStaged || (rec.Header().Get(listVersionHeader) == `"after-copy"`) != test.wantStaged {
				t.Errorf("post-copy list reads = %d, version %q", listReads.Load(), rec.Header().Get(listVersionHeader))
			}
		})
	}
}

func TestMailboxProxy_AdvertisesNativeCopyOnItsInbox(t *testing.T) {
	in, _ := connected(t, &fakeAgnosticAI{})
	in.writeClipboard = func(context.Context, string) bool { return true }
	status, body := getInbox(t, in, "")
	if status != http.StatusOK || !strings.Contains(body, `"mailboxNativeCopy":true`) {
		t.Errorf("inbox = %d %s", status, body)
	}
}

func TestMailboxProxy_RejectsConfirmationWithoutCopying(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	in.writeClipboard = func(context.Context, string) bool {
		t.Error("confirm-only request touched the clipboard")
		return true
	}

	req := httptest.NewRequest(http.MethodPost, serverMailboxStagePath, strings.NewReader(`{"thread":"some-id","n":1,"how":"copy","confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || len(fake.posts) != 0 {
		t.Errorf("confirm-only request = %d %s, upstream writes %v", rec.Code, rec.Body, fake.posts)
	}
}

func TestMailboxProxy_ForwardsReviewRefsWithoutRefreshingTheList(t *testing.T) {
	const payload = `{"tool":"claude","id":"some-id","base":"main"}`
	const refs = `{"worktree":"/repo","base":"abc","commit":"def","paths":["src/main.go"],"uncommitted":1}`
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{
		"/api/mailbox/refs": {status: http.StatusOK, body: refs},
	}}
	in, _ := connected(t, fake)
	req := httptest.NewRequest(http.MethodPost, "/api/mailbox/refs", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != refs ||
		rec.Header().Get(listVersionHeader) != "" || fake.reads != 0 {
		t.Errorf("review refs = %d %s, version %q, list reads %d", rec.Code, rec.Body, rec.Header().Get(listVersionHeader), fake.reads)
	}
	posts := fake.posts["/api/mailbox/refs"]
	if len(posts) != 1 || posts[0] != "application/json "+payload {
		t.Errorf("forwarded review refs = %v", posts)
	}
}

func TestMailboxProxy_ForwardsClaimsAndRefreshesAnOwnershipConflict(t *testing.T) {
	const refusal = `{"guard":"ownership","error":"The path is held by another session."}`
	var claimsReads, listReads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/mailbox/claims":
			count := claimsReads.Add(1)
			if r.Method != http.MethodGet || count == 1 && r.URL.RawQuery != "" ||
				count == 2 && (r.URL.Query().Get("tool") != "claude" || r.URL.Query().Get("id") != "some-id") {
				t.Errorf("claims read = %s %s", r.Method, r.URL)
			}
			_, _ = w.Write([]byte(`{"claims":[],"conflicts":[]}`))
		case "/api/mailbox/claim", "/api/mailbox/release", "/api/mailbox/handoff":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("mutation = %s %s", r.Method, r.URL)
			}
			if r.URL.Path == "/api/mailbox/claim" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(refusal))
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		case serverListPath:
			listReads.Add(1)
			w.Header().Set("ETag", `"after-claim"`)
			writeJSON(w, http.StatusOK, map[string]any{"needsYou": 0})
		default:
			t.Errorf("unexpected upstream path %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := newServer()
	server.base, server.health.Version = upstream.URL, "1.2.3"
	in := newInbox(server, "1.2.3", func() {})
	routes := (&shell{}).routes(in, fstest.MapFS{})

	for _, path := range []string{"/api/mailbox/claims", "/api/mailbox/claims?tool=claude&id=some-id"} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != `{"claims":[],"conflicts":[]}` {
			t.Errorf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
	for _, action := range []string{"claim", "release", "handoff"} {
		req := httptest.NewRequest(http.MethodPost, "/api/mailbox/"+action, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if action == "claim" {
			if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != refusal {
				t.Errorf("claim refusal = %d %s", rec.Code, rec.Body)
			}
		} else if rec.Code != http.StatusNoContent {
			t.Errorf("%s = %d %s", action, rec.Code, rec.Body)
		}
		if rec.Header().Get(listVersionHeader) != `"after-claim"` {
			t.Errorf("%s did not refresh the list: version %q", action, rec.Header().Get(listVersionHeader))
		}
	}
	if claimsReads.Load() != 2 || listReads.Load() != 3 {
		t.Errorf("claims and list reads = %d and %d", claimsReads.Load(), listReads.Load())
	}
}
