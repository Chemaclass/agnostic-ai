package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func copied(in *inbox, text string) {
	in.readClipboard = func() string { return text }
}

func sendFrom(t *testing.T, in *inbox, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(body)))
	return rec
}

func TestClipboard_DescribesTheCopiedTextWithoutAllOfIt(t *testing.T) {
	in, _ := connected(t, &fakeAgnosticAI{})
	long := strings.Repeat("é", 150) + " " + strings.Repeat("b", 150)
	copied(in, long)

	rec := httptest.NewRecorder()
	(&shell{}).routes(in, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/clipboard", nil))

	var reply clipboardReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || reply.Preview != long[:len(strings.Repeat("é", 150))+1+49] || reply.Words != 2 || reply.Chars != 301 || reply.Hash != clipboardHash(long) {
		t.Errorf("clipboard = %d %+v", rec.Code, reply)
	}
}

func TestClipboard_SaysNothingForEmptyText(t *testing.T) {
	for _, text := range []string{"", " \n\t "} {
		if reply := describeClipboard(text); reply != (clipboardReply{}) {
			t.Errorf("describeClipboard(%q) = %+v, want nothing", text, reply)
		}
	}
}

func TestSend_ForwardsTheClipboardTheBannerShowed(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusOK, body: `{"sent":true,"message":"Typed into the Ghostty tab that runs it. Press Enter there to send it."}`}}}
	in, _ := connected(t, fake)
	text := "one\ntwo <b>&</b>\nthree"
	copied(in, text)

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+clipboardHash(text)+`"}`)

	if rec.Code != http.StatusOK || rec.Body.String() != `{"sent":true,"message":"Typed into the Ghostty tab that runs it. Press Enter there to send it."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts[serverSendPath]; len(sent) != 1 || sent[0] != `application/json {"tool":"claude","id":"a","text":"one\ntwo <b>&</b>\nthree"}` {
		t.Errorf("the server received %q", sent)
	}
	select {
	case <-in.refresh:
	default:
		t.Error("a send the server took asked for no new list")
	}
}

func TestSend_RefusesWithoutAskingTheServer(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	shown := "the text the banner showed"
	for name, tc := range map[string]struct {
		clipboard, body string
		status          int
		reply           string
	}{
		"clipboard changed": {clipboard: "something copied later", body: `{"tool":"claude","id":"a","hash":"` + clipboardHash(shown) + `"}`, status: http.StatusConflict, reply: `{"error":"The copied text changed. Check the new text, then send again."}`},
		"clipboard emptied": {clipboard: "", body: `{"tool":"claude","id":"a","hash":"` + clipboardHash(shown) + `"}`, status: http.StatusConflict, reply: `{"error":"Nothing to send: copy some text first."}`},
		"text over the limit": {
			clipboard: strings.Repeat("a", maxSendBytes+1),
			body:      `{"tool":"claude","id":"a","hash":"` + clipboardHash(strings.Repeat("a", maxSendBytes+1)) + `"}`,
			status:    http.StatusRequestEntityTooLarge,
			reply:     `{"error":"The text is 257 KB, over the 256 KB a send takes, so nothing was typed."}`,
		},
		"text that escapes past the server's limit": {
			clipboard: strings.Repeat("\x01", 200<<10),
			body:      `{"tool":"claude","id":"a","hash":"` + clipboardHash(strings.Repeat("\x01", 200<<10)) + `"}`,
			status:    http.StatusRequestEntityTooLarge,
			reply:     `{"error":"The text holds too many special characters to send, so nothing was typed. Copy less of it."}`,
		},
		"no hash":          {clipboard: shown, body: `{"tool":"claude","id":"a"}`, status: http.StatusBadRequest, reply: `{"error":"send {tool, id, hash}"}`},
		"the text itself":  {clipboard: shown, body: `{"tool":"claude","id":"a","hash":"x","text":"rm -rf ~"}`, status: http.StatusBadRequest, reply: `{"error":"send {tool, id, hash}"}`},
		"not a JSON value": {clipboard: shown, body: `tool=claude`, status: http.StatusBadRequest, reply: `{"error":"send {tool, id, hash}"}`},
	} {
		t.Run(name, func(t *testing.T) {
			copied(in, tc.clipboard)
			rec := sendFrom(t, in, tc.body)
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
		})
	}
	if len(fake.posts[serverSendPath]) != 0 {
		t.Errorf("the server was asked: %q", fake.posts[serverSendPath])
	}
}

func TestSend_PassesTheServersRefusalBack(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	copied(in, "gone")

	rec := sendFrom(t, in, `{"tool":"claude","id":"gone","hash":"`+clipboardHash("gone")+`"}`)

	if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}

func TestSend_SaysToUpgradeAServerWithoutTheEndpoint(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusNotFound, body: `404 page not found`}}}
	in, _ := connected(t, fake)
	copied(in, "hello")

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+clipboardHash("hello")+`"}`)

	if rec.Code != http.StatusNotImplemented || strings.TrimSpace(rec.Body.String()) != `{"error":"The installed agnostic-ai cannot send text yet. Run: agnostic-ai companion install, or upgrade agnostic-ai."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}
