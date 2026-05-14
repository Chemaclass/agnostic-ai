package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// hookSlugMaxLen caps the matcher slug at a length that keeps filenames
// readable on every common filesystem after appending the 8-char hash.
const hookSlugMaxLen = 24

// hookSpecName returns a deterministic spec filename stem for a hook
// derived from event, matcher, and its commands. Same inputs produce the
// same name across imports, so user edits to a hook spec survive a
// re-import: the upstream hook resolves to the same path and overwrites
// itself instead of clobbering an unrelated file at the same position.
//
// Format: "<event-lower>[-<matcher-slug>]-<hash8>". The matcher segment
// is omitted when the slug is empty (matcher unset or pure punctuation).
func hookSpecName(event, matcher string, commands []string) string {
	ev := strings.ToLower(strings.TrimSpace(event))
	if ev == "" {
		ev = "hook"
	}
	slug := hookMatcherSlug(matcher)
	hash := hookContentHash(event, matcher, commands)
	if slug == "" {
		return ev + "-" + hash
	}
	return ev + "-" + slug + "-" + hash
}

// hookMatcherSlug lowercases the matcher, collapses any non-alphanumeric
// run into a single dash, and trims leading and trailing dashes. The
// result is capped at hookSlugMaxLen. Returns empty if the matcher had
// no alphanumeric content.
func hookMatcherSlug(matcher string) string {
	matcher = strings.ToLower(matcher)
	var b strings.Builder
	b.Grow(len(matcher))
	prevDash := true
	for _, r := range matcher {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > hookSlugMaxLen {
		slug = strings.TrimRight(slug[:hookSlugMaxLen], "-")
	}
	return slug
}

// hookContentHash returns the first 8 hex chars of SHA-256 over a
// canonical "event|matcher|<cmd1>\x00<cmd2>..." string. Commands are
// sorted so the order in source files does not affect the hash.
func hookContentHash(event, matcher string, commands []string) string {
	sorted := append([]string(nil), commands...)
	sort.Strings(sorted)
	canon := event + "|" + matcher + "|" + strings.Join(sorted, "\x00")
	sum := sha256.Sum256([]byte(canon))
	return hex.EncodeToString(sum[:])[:8]
}
