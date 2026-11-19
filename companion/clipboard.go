package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
)

const serverSendPath = "/api/conversations/send"

// maxSendBytes is the server's limit on the text one send types.
const maxSendBytes = 256 << 10

// maxServerBodyBytes is the most the server reads of a request: JSON
// escapes a control character as six bytes, so text under maxSendBytes
// can still pass it.
const maxServerBodyBytes = 1 << 20

// previewRunes is how much of the copied text the pick banner shows.
const previewRunes = 200

// clipboardReply describes the copied text without carrying all of it.
// hash names the exact text, so a send can tell it did not change.
type clipboardReply struct {
	Preview string `json:"preview"`
	Words   int    `json:"words"`
	Chars   int    `json:"chars"`
	Hash    string `json:"hash"`
}

type sendRequest struct {
	Tool string `json:"tool"`
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

// copiedHash names the text and the HTML copied with it, so a send can
// tell that neither changed and that both came from one copy.
func copiedHash(text, html string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d\x00%s%d\x00%s", len(text), text, len(html), html))
	return hex.EncodeToString(sum[:])
}

func describeClipboard(text, html string) clipboardReply {
	if strings.TrimSpace(text) == "" {
		return clipboardReply{}
	}
	preview := text
	if utf8.RuneCountInString(preview) > previewRunes {
		preview = string([]rune(preview)[:previewRunes])
	}
	return clipboardReply{Preview: preview, Words: len(strings.Fields(text)), Chars: utf8.RuneCountInString(text), Hash: copiedHash(text, html)}
}

func (in *inbox) copiedHTML() string {
	if in.readClipboardHTML == nil {
		return ""
	}
	return in.readClipboardHTML()
}

// clipboard answers GET /api/clipboard with what the person copied.
func (in *inbox) clipboard(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, describeClipboard(in.readClipboard(), in.copiedHTML()))
}

// send answers POST /api/send. The page names a conversation and the
// hash of the text it showed; the shell reads the clipboard itself, so
// the page never carries the text, and a clipboard that changed since is
// refused. The server checks the rest and types it.
func (in *inbox) send(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.Hash == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {tool, id, hash}"})
		return
	}
	text, html := in.readClipboard(), in.copiedHTML()
	if strings.TrimSpace(text) == "" {
		writeJSON(w, http.StatusConflict, apiError{Error: "Nothing to send: copy some text first."})
		return
	}
	if copiedHash(text, html) != req.Hash {
		writeJSON(w, http.StatusConflict, apiError{Error: "The copied text changed. Check the new text, then send again."})
		return
	}
	if len(text) > maxSendBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, apiError{Error: fmt.Sprintf("The text is %d KB, over the %d KB a send takes, so nothing was typed.", (len(text)+1023)>>10, maxSendBytes>>10)})
		return
	}
	body, err := serverSendBody(req, text, html)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	if len(body) > maxServerBodyBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, apiError{Error: "The text holds too many special characters to send, so nothing was typed. Copy less of it."})
		return
	}
	answer, err := in.server.callWithin(r.Context(), in.server.sendWait, http.MethodPost, serverSendPath, body, "")
	if mayHaveArrived(err) {
		status := http.StatusBadGateway
		message := "agnostic-ai stopped answering. The text may have been typed. Check the chat before sending again."
		if timedOut(err) {
			status = http.StatusGatewayTimeout
			message = "agnostic-ai took too long to answer. The text may have been typed. Check the chat before sending again."
		}
		writeJSON(w, status, apiError{Error: message})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	if missingEndpoint(answer) {
		writeJSON(w, http.StatusNotImplemented, apiError{Error: "The installed agnostic-ai cannot send text yet. Run: agnostic-ai companion install, or upgrade agnostic-ai."})
		return
	}
	if answer.status < 300 {
		in.refreshSoon()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(answer.status)
	_, _ = io.Copy(w, bytes.NewReader(answer.body))
}

// mayHaveArrived reports whether err is a call that got no answer after
// it may have reached the server: anything but a failed connect.
func mayHaveArrived(err error) bool {
	if _, ok := errors.AsType[*callError](err); !ok {
		return false
	}
	var dial *net.OpError
	return !errors.As(err, &dial) || dial.Op != "dial"
}

// missingEndpoint reports whether the server answered as a release older
// than the endpoint does: Go's own 404 page, or the conversations
// handler's "no endpoint at" for a path it does not know.
func missingEndpoint(answer reply) bool {
	switch answer.status {
	case http.StatusMethodNotAllowed:
		return true
	case http.StatusNotFound:
		return strings.TrimSpace(string(answer.body)) == "404 page not found" || strings.HasPrefix(errorOf(answer), "no endpoint at ")
	}
	return false
}

// serverSendBody is the server's send request. HTML that would push it
// past the server's limit is left out, so the plain text still goes.
func serverSendBody(req sendRequest, text, html string) ([]byte, error) {
	body, err := encodeSend(req, text, html)
	if err != nil || html == "" || len(body) <= maxServerBodyBytes {
		return body, err
	}
	return encodeSend(req, text, "")
}

func encodeSend(req sendRequest, text, html string) ([]byte, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(struct {
		Tool string `json:"tool"`
		ID   string `json:"id"`
		Text string `json:"text"`
		HTML string `json:"html,omitempty"`
	}{req.Tool, req.ID, text, html}); err != nil {
		return nil, fmt.Errorf("encode the send: %w", err)
	}
	return bytes.TrimSuffix(body.Bytes(), []byte("\n")), nil
}
