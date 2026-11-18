package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
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
	if rec.Code != http.StatusOK || reply.Preview != long[:len(strings.Repeat("é", 150))+1+49] || reply.Words != 2 || reply.Chars != 301 || reply.Hash != copiedHash(long, "") {
		t.Errorf("clipboard = %d %+v", rec.Code, reply)
	}
}

func TestClipboard_SaysNothingForEmptyText(t *testing.T) {
	for _, text := range []string{"", " \n\t "} {
		if reply := describeClipboard(text, "<h1>Title</h1>"); reply != (clipboardReply{}) {
			t.Errorf("describeClipboard(%q) = %+v, want nothing", text, reply)
		}
	}
}

func TestClipboard_NamesNoFormatTheServerHasNotChosen(t *testing.T) {
	body, err := json.Marshal(describeClipboard("see docs", `<ul><li>see <a href="https://a.example">docs</a></li></ul>`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "format") {
		t.Errorf("clipboard = %s, want no format: only the server knows whether Markdown goes", body)
	}
}

func TestCopiedHash_TellsTextFromHTML(t *testing.T) {
	if copiedHash("5\x00helloWORLD", "") == copiedHash("hello", "WORLD") {
		t.Error("text alone hashed like other text with HTML")
	}
	if copiedHash("ab", "") == copiedHash("a", "b") {
		t.Error("the HTML's start was read as the text's end")
	}
}

func TestSend_ForwardsTheCopiedHTMLBesideTheText(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusOK, body: `{"sent":true,"message":"Typed the Markdown into the Ghostty tab that runs it. Press Enter there to send it."}`}}}
	in, _ := connected(t, fake)
	copied(in, "Notes")
	in.readClipboardHTML = func() string { return "<h1>Notes</h1>" }

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("Notes", "<h1>Notes</h1>")+`"}`)

	if rec.Code != http.StatusOK {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts[serverSendPath]; len(sent) != 1 || sent[0] != `application/json {"tool":"claude","id":"a","text":"Notes","html":"<h1>Notes</h1>"}` {
		t.Errorf("the server received %q", sent)
	}
}

func TestSend_LeavesOutHTMLTooLargeForTheServer(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusOK, body: `{"sent":true,"message":"ok"}`}}}
	in, _ := connected(t, fake)
	copied(in, "Notes")
	huge := "<h1>" + strings.Repeat("a", maxServerBodyBytes) + "</h1>"
	in.readClipboardHTML = func() string { return huge }

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("Notes", huge)+`"}`)

	if rec.Code != http.StatusOK {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
	if sent := fake.posts[serverSendPath]; len(sent) != 1 || sent[0] != `application/json {"tool":"claude","id":"a","text":"Notes"}` {
		t.Errorf("the server received %.200q", sent)
	}
}

func TestSend_RefusesHTMLFromAnotherCopy(t *testing.T) {
	fake := &fakeAgnosticAI{}
	in, _ := connected(t, fake)
	copied(in, "Notes")
	shown := describeClipboard("Notes", "<h1>Notes</h1>").Hash
	in.readClipboardHTML = func() string { return "<ul><li>something else</li></ul>" }

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+shown+`"}`)

	if rec.Code != http.StatusConflict || len(fake.posts[serverSendPath]) != 0 {
		t.Errorf("send = %d %s, server asked %q", rec.Code, rec.Body, fake.posts[serverSendPath])
	}
}

func TestSend_ForwardsTheClipboardTheBannerShowed(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusOK, body: `{"sent":true,"message":"Typed into the Ghostty tab that runs it. Press Enter there to send it."}`}}}
	in, _ := connected(t, fake)
	text := "one\ntwo <b>&</b>\nthree"
	copied(in, text)

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash(text, "")+`"}`)

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
		"clipboard changed": {clipboard: "something copied later", body: `{"tool":"claude","id":"a","hash":"` + copiedHash(shown, "") + `"}`, status: http.StatusConflict, reply: `{"error":"The copied text changed. Check the new text, then send again."}`},
		"clipboard emptied": {clipboard: "", body: `{"tool":"claude","id":"a","hash":"` + copiedHash(shown, "") + `"}`, status: http.StatusConflict, reply: `{"error":"Nothing to send: copy some text first."}`},
		"text over the limit": {
			clipboard: strings.Repeat("a", maxSendBytes+1),
			body:      `{"tool":"claude","id":"a","hash":"` + copiedHash(strings.Repeat("a", maxSendBytes+1), "") + `"}`,
			status:    http.StatusRequestEntityTooLarge,
			reply:     `{"error":"The text is 257 KB, over the 256 KB a send takes, so nothing was typed."}`,
		},
		"text that escapes past the server's limit": {
			clipboard: strings.Repeat("\x01", 200<<10),
			body:      `{"tool":"claude","id":"a","hash":"` + copiedHash(strings.Repeat("\x01", 200<<10), "") + `"}`,
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

	rec := sendFrom(t, in, `{"tool":"claude","id":"gone","hash":"`+copiedHash("gone", "")+`"}`)

	if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}

func TestSend_SaysToUpgradeAServerWithoutTheEndpoint(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusNotFound, body: `404 page not found`}}}
	in, _ := connected(t, fake)
	copied(in, "hello")

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("hello", "")+`"}`)

	if rec.Code != http.StatusNotImplemented || strings.TrimSpace(rec.Body.String()) != `{"error":"The installed agnostic-ai cannot send text yet. Run: agnostic-ai companion install, or upgrade agnostic-ai."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}

func TestSend_SaysTheTextMayHaveBeenTypedWhenTheServerIsLate(t *testing.T) {
	fake := &fakeAgnosticAI{hold: make(chan struct{})}
	in, _ := connected(t, fake)
	in.server.sendWait = 50 * time.Millisecond
	copied(in, "hello")

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("hello", "")+`"}`)

	if rec.Code != http.StatusGatewayTimeout || strings.TrimSpace(rec.Body.String()) != `{"error":"agnostic-ai took too long to answer. It may have been typed. Check the chat before sending again."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}

func TestSend_PassesAReworded404Back(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusNotFound, body: `{"error":"This chat left the list. Refresh the page."}`}}}
	in, _ := connected(t, fake)
	copied(in, "hello")

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("hello", "")+`"}`)

	if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"This chat left the list. Refresh the page."}` {
		t.Errorf("send = %d %s; want the server's own answer, not a call to upgrade", rec.Code, rec.Body)
	}
}

func TestSend_SaysToUpgradeWhenTheConversationsHandlerKnowsNoSend(t *testing.T) {
	fake := &fakeAgnosticAI{answers: map[string]fakeAnswer{serverSendPath: {status: http.StatusNotFound, body: `{"error":"no endpoint at /api/conversations/send"}`}}}
	in, _ := connected(t, fake)
	copied(in, "hello")

	rec := sendFrom(t, in, `{"tool":"claude","id":"a","hash":"`+copiedHash("hello", "")+`"}`)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
}
