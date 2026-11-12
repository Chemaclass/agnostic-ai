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

// waiting is what needs the person in one list: the blocked and unread
// conversations it shows, and how many need the person of every age.
type waiting struct {
	ids   map[string]bool
	count int
}

// waitingIn reads what waits from a list as the server sent it. It
// reports false for a list it cannot read.
func waitingIn(list json.RawMessage) (waiting, bool) {
	var view struct {
		NeedsYou      int `json:"needsYou"`
		Conversations []struct {
			Tool  string `json:"tool"`
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	if len(list) == 0 || json.Unmarshal(list, &view) != nil {
		return waiting{}, false
	}
	now := waiting{ids: map[string]bool{}, count: view.NeedsYou}
	for _, conversation := range view.Conversations {
		if conversation.State == "blocked" || conversation.State == "unread" {
			now.ids[conversation.Tool+":"+conversation.ID] = true
		}
	}
	return now, true
}

// gained reports whether something waits now that did not in before: a
// conversation that was not waiting, even while another stopped, or a
// higher count, which also covers one older than the list shows.
func (w waiting) gained(before waiting) bool {
	if w.count > before.count {
		return true
	}
	for id := range w.ids {
		if !before.ids[id] {
			return true
		}
	}
	return false
}
