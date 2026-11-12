package main

import "encoding/json"

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
// conversations, by tool and id. The list holds every one of any age.
type waiting map[string]bool

// waitingIn reads what waits from a list as the server sent it. It
// reports false for a list it cannot read, and for one that says its
// marks or its cloud sessions could not be read: what waits there is
// not what waits, and taking it as the last list would make the next
// full one look new.
func waitingIn(list json.RawMessage) (waiting, bool) {
	var view struct {
		MarksNote     string `json:"marksNote"`
		CloudNote     string `json:"cloudNote"`
		Conversations []struct {
			Tool  string `json:"tool"`
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	if len(list) == 0 || json.Unmarshal(list, &view) != nil || view.MarksNote != "" || view.CloudNote != "" {
		return nil, false
	}
	now := waiting{}
	for _, conversation := range view.Conversations {
		if conversation.State == "blocked" || conversation.State == "unread" {
			now[conversation.Tool+":"+conversation.ID] = true
		}
	}
	return now, true
}

// gained reports whether a conversation waits now that did not in
// before, even while another stopped waiting.
func (w waiting) gained(before waiting) bool {
	for id := range w {
		if !before[id] {
			return true
		}
	}
	return false
}
