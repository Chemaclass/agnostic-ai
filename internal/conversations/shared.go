package conversations

import (
	"regexp"
	"strings"
	"time"
)

// bridgeSessionPrefix starts the id a Claude Code process writes as
// bridgeSessionId while Remote Control shares it. claude.ai lists the
// same session as cse_<id>. The relation is undocumented: it was seen on
// one account on 2026-11-12, and when it stops holding both rows show.
const bridgeSessionPrefix = "session_"

var bridgeSessionPattern = regexp.MustCompile(`^session_[A-Za-z0-9_-]{1,128}$`)

// bridgeCloudID is the cloud session id a bridge id names, when it is one.
func bridgeCloudID(bridge string) (string, bool) {
	if !bridgeSessionPattern.MatchString(bridge) {
		return "", false
	}
	return cloudSessionIDPrefix + strings.TrimPrefix(bridge, bridgeSessionPrefix), true
}

// shareCloud folds each cloud row that a live local session also holds
// into that local row, which keeps its own state and route and gains
// the cloud session's page. A cloud id two live sessions claim is left
// apart, as is every row without a match.
func shareCloud(rows []Conversation) []Conversation {
	var claims map[string]int
	for i, c := range rows {
		if !c.Live || c.App == AppCloud || c.bridge == "" {
			continue
		}
		id, ok := bridgeCloudID(c.bridge)
		if !ok {
			continue
		}
		if claims == nil {
			claims = map[string]int{}
		}
		if _, taken := claims[id]; taken {
			claims[id] = -1
			continue
		}
		claims[id] = i
	}
	return takeIn(rows, claims)
}

// takeIn folds each cloud row claims names into the local row at its
// index, which keeps its own state and route and gains the cloud
// session's page. A claim of -1 folds nothing.
func takeIn(rows []Conversation, claims map[string]int) []Conversation {
	if claims == nil {
		return rows
	}
	var taken map[int]bool
	for i, c := range rows {
		at, ok := claims[c.ID]
		if !ok || at < 0 || c.App != AppCloud {
			continue
		}
		cloud := c
		cloud.heldBy = rows[at].ID
		rows[at].Cloud = &CloudRef{ID: c.ID, Link: c.Open.Link, row: &cloud}
		if taken == nil {
			taken = map[int]bool{}
		}
		taken[i] = true
	}
	if taken == nil {
		return rows
	}
	shared := rows[:0]
	for i, c := range rows {
		if !taken[i] {
			shared = append(shared, c)
		}
	}
	return shared
}

// maxTwins bounds the links between local sessions and their cloud
// twins that the scanner remembers.
const maxTwins = 1024

// twinEchoSpan is how long, on the cloud's clock, after the last cloud
// write the scanner saw while the session ran, the twin may still write
// that session's last reply: the cloud list is asked at most once per
// freshSpan, so the read before the session closed can miss it. The
// folded row shows a write in that span as its own, unread until seen;
// a later write is new work on the twin's own row.
const twinEchoSpan = 2 * freshSpan

// twin is the cloud row a local session took in while it ran.
type twin struct {
	cloud string
	// written is the cloud row's last write read while the session ran.
	written time.Time
}

// followTwins keeps a local session and its cloud twin as one row once
// the session ends. It remembers the cloud row each live shared row
// took in, and folds that row into the local row again while the
// session no longer runs, as long as the cloud row holds no write past
// twinEchoSpan after the ones the scanner saw: a later write came from
// the cloud side and shows on its own row. The folded row takes the
// twin's last write when it is later than its own. The link lives in
// memory only, so a new server lists both rows again. A live session that says it has no Remote
// Control id ends the link. Only a complete read, whole is true, drops
// a link whose local or cloud row is gone.
func (s *Scanner) followTwins(rows []Conversation, whole bool) []Conversation {
	for _, c := range rows {
		key := markKey(c.Tool, c.ID)
		switch {
		case c.Cloud != nil:
			s.rememberTwin(key, c.Cloud.ID, lastWritten(*c.Cloud.row))
		case c.Live && c.bridge == "":
			delete(s.twins, key)
		}
	}
	if len(s.twins) == 0 {
		return rows
	}
	clouds := map[string]int{}
	locals := map[string]bool{}
	for i, c := range rows {
		switch {
		case c.App == AppCloud:
			clouds[c.ID] = i
		case c.Cloud != nil:
			clouds[c.Cloud.ID] = -1
			locals[markKey(c.Tool, c.ID)] = true
		default:
			locals[markKey(c.Tool, c.ID)] = true
		}
	}
	if whole {
		for key, link := range s.twins {
			if _, listed := clouds[link.cloud]; !listed || !locals[key] {
				delete(s.twins, key)
			}
		}
	}
	var claims map[string]int
	for i, c := range rows {
		if c.Live || c.App == AppCloud || c.Cloud != nil {
			continue
		}
		link, known := s.twins[markKey(c.Tool, c.ID)]
		if !known {
			continue
		}
		at, listed := clouds[link.cloud]
		if !listed || at < 0 {
			continue
		}
		if lastWritten(rows[at]).After(link.written.Add(twinEchoSpan)) {
			continue
		}
		if claims == nil {
			claims = map[string]int{}
		}
		if _, taken := claims[link.cloud]; taken {
			claims[link.cloud] = -1
			continue
		}
		claims[link.cloud] = i
	}
	rows = takeIn(rows, claims)
	for i := range rows {
		c := &rows[i]
		if !c.Live && c.Cloud != nil && lastWritten(*c.Cloud.row).After(lastWritten(*c)) {
			c.UpdatedAt = c.Cloud.row.UpdatedAt
		}
	}
	return rows
}

// rememberTwin links a local row's mark key to the cloud row it took
// in. A full map gives up the link whose cloud row was written longest ago.
func (s *Scanner) rememberTwin(key, cloud string, written time.Time) {
	link, known := s.twins[key]
	if known && link.cloud == cloud && link.written.After(written) {
		written = link.written
	}
	if !known && len(s.twins) >= maxTwins {
		oldest := ""
		for other, l := range s.twins {
			if oldest == "" || l.written.Before(s.twins[oldest].written) {
				oldest = other
			}
		}
		delete(s.twins, oldest)
	}
	if s.twins == nil {
		s.twins = map[string]twin{}
	}
	s.twins[key] = twin{cloud: cloud, written: written}
}
