package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

func clipboardHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func describeClipboard(text string) clipboardReply {
	if strings.TrimSpace(text) == "" {
		return clipboardReply{}
	}
	preview := text
	if utf8.RuneCountInString(preview) > previewRunes {
		preview = string([]rune(preview)[:previewRunes])
	}
	return clipboardReply{Preview: preview, Words: len(strings.Fields(text)), Chars: utf8.RuneCountInString(text), Hash: clipboardHash(text)}
}

// clipboard answers GET /api/clipboard with what the person copied.
func (in *inbox) clipboard(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, describeClipboard(in.readClipboard()))
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
	text := in.readClipboard()
	if strings.TrimSpace(text) == "" {
		writeJSON(w, http.StatusConflict, apiError{Error: "Nothing to send: copy some text first."})
		return
	}
	if clipboardHash(text) != req.Hash {
		writeJSON(w, http.StatusConflict, apiError{Error: "The copied text changed. Check the new text, then send again."})
		return
	}
	if len(text) > maxSendBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, apiError{Error: fmt.Sprintf("The text is %d KB, over the %d KB a send takes, so nothing was typed.", (len(text)+1023)>>10, maxSendBytes>>10)})
		return
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(struct {
		Tool string `json:"tool"`
		ID   string `json:"id"`
		Text string `json:"text"`
	}{req.Tool, req.ID, text}); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	if body.Len() > maxServerBodyBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, apiError{Error: "The text holds too many special characters to send, so nothing was typed. Copy less of it."})
		return
	}
	answer, err := in.server.call(r.Context(), http.MethodPost, serverSendPath, bytes.TrimSuffix(body.Bytes(), []byte("\n")), "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, apiError{Error: err.Error()})
		return
	}
	// A release older than the send endpoint answers its path as an unknown one.
	if (answer.status == http.StatusNotFound || answer.status == http.StatusMethodNotAllowed) && !strings.HasPrefix(errorOf(answer), "That conversation") {
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
