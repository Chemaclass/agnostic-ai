package conversations

import (
	"regexp"
	"strings"
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
