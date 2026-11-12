package main

import (
	"encoding/json"
	"strconv"
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
// conversations (the list holds every one of any age), and how well the
// list could read each place they come from.
type waiting struct {
	// ids holds each waiting conversation, by tool and id, with its source.
	ids map[string]string
	// health says, for each source, what could not be read there: a
	// tool's store, or the cloud sessions. Empty when all of it was read.
	health map[string]string
	// marks says why the marks could not be read; it changes every row.
	marks string
}

// cloudSource is where cloud sessions come from, whatever their tool.
const cloudSource = "cloud"

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
			App   string `json:"app"`
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	if len(list) == 0 || json.Unmarshal(list, &view) != nil {
		return waiting{}, false
	}
	now := waiting{ids: map[string]string{}, health: map[string]string{cloudSource: view.CloudNote}, marks: view.MarksNote}
	for _, tool := range view.Tools {
		now.health[tool.Tool] = strconv.FormatBool(tool.Found) + ":" + tool.Note
	}
	for _, conversation := range view.Conversations {
		if conversation.State != "blocked" && conversation.State != "unread" {
			continue
		}
		source := conversation.Tool
		if conversation.App == cloudSource {
			source = cloudSource
		}
		now.ids[conversation.Tool+":"+conversation.ID] = source
	}
	return now, true
}

// gained reports whether a conversation waits now that did not in
// before, even while another stopped waiting. A source read differently
// in the two lists is not compared: what it read more would look new.
func (w waiting) gained(before waiting) bool {
	if w.marks != before.marks {
		return false
	}
	for id, source := range w.ids {
		if w.health[source] != before.health[source] {
			continue
		}
		if _, was := before.ids[id]; !was {
			return true
		}
	}
	return false
}
