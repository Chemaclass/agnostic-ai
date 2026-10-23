package spec

import (
	"regexp"
	"testing"
)

func TestCodebuddyPortableMatchersDoNotMatchToolSubstrings(t *testing.T) {
	for _, c := range []struct{ kind, positive, negative string }{{"shell", "Bash", "mcp__docs__BashLookup"}, {"edit", "Edit", "ReEditHistory"}, {"edit", "Write", "mcp__docs__WriteNotes"}, {"read", "Read", "mcp__docs__ReadNotes"}, {"web", "WebFetch", "mcp__docs__WebFetchProxy"}, {"mcp:docs", "mcp__docs__search", "prefix_mcp__docs__search"}} {
		matcher, ok := HookToolMatcher("codebuddy", c.kind)
		if !ok {
			t.Fatal(c.kind)
		}
		re, err := regexp.Compile(matcher)
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(c.positive) || re.MatchString(c.negative) {
			t.Errorf("%s matcher %q matched an unrelated tool or missed the intended tool", c.kind, matcher)
		}
	}
}
