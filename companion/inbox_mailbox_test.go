package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
