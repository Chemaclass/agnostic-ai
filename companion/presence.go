package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// presence is how the companion sits on the screen. It is never saved:
// a restart starts shown.
type presence string

const (
	// shown is the circle at full color, as usual.
	shown presence = "shown"
	// hidden is no circle and no list. The companion keeps reading.
	hidden presence = "hidden"
	// fadedBack is the circle back on its own at 25%, with its count,
	// because something new waits for the person while it was hidden.
	fadedBack presence = "faded"
)

// waiting is what needs the person in one list: its blocked and unread
// conversations by tool and id (the list holds every one of any age),
// and what the list says it could not read.
type waiting struct {
	ids map[string]bool
	// gaps names the marks, cloud sessions and tool stores the list could
	// not read. Two lists with different gaps saw different conversations.
	gaps string
}

// waitingIn reads what waits from a list as the server sent it. It
// reports false for a list it cannot read.
func waitingIn(list json.RawMessage) (waiting, bool) {
	var view struct {
		MarksNote string `json:"marksNote"`
		CloudNote string `json:"cloudNote"`
		Tools     []struct {
			Tool  string `json:"tool"`
			Found bool   `json:"found"`
			Note  string `json:"note"`
		} `json:"tools"`
		Conversations []struct {
			Tool  string `json:"tool"`
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	if len(list) == 0 || json.Unmarshal(list, &view) != nil {
		return waiting{}, false
	}
	gaps := []string{"marks:" + view.MarksNote, "cloud:" + view.CloudNote}
	for _, tool := range view.Tools {
		gaps = append(gaps, tool.Tool+":"+strconv.FormatBool(tool.Found)+":"+tool.Note)
	}
	now := waiting{ids: map[string]bool{}, gaps: strings.Join(gaps, "\n")}
	for _, conversation := range view.Conversations {
		if conversation.State == "blocked" || conversation.State == "unread" {
			now.ids[conversation.Tool+":"+conversation.ID] = true
		}
	}
	return now, true
}

// gained reports whether a conversation waits now that did not in
// before, even while another stopped waiting. Lists that could not read
// the same things are not compared: the fuller one would look new.
func (w waiting) gained(before waiting) bool {
	if w.gaps != before.gaps {
		return false
	}
	for id := range w.ids {
		if !before.ids[id] {
			return true
		}
	}
	return false
}
