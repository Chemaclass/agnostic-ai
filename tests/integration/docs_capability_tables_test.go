package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The capability tables in docs/user/targets.md and README.md are edited
// by hand, and parallel target-audit PRs kept landing both sides of a
// change instead of merging them. That left duplicated rows whose cells
// disagreed: one trae row claimed no MCP surface while the next gave
// `.trae/mcp.json`, and one qoder row claimed no skills while the next
// gave `.qoder/skills/<name>/SKILL.md`. Each pair had one current and one
// stale row, and not consistently in the same position, so a reader had
// no way to tell which to believe (#597).
//
// A duplicate row is always a mistake here: these tables carry one row
// per target. Assert that directly, so the next merge that lands both
// sides fails the build instead of shipping a contradiction.

var docTableRow = regexp.MustCompile(`^\|\s*(?:\*\*)?([A-Za-z][A-Za-z0-9 ()/.-]*?)(?:\*\*)?\s*\|`)

func TestDocs_CapabilityTablesHaveNoDuplicateRows(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"docs/user/targets.md", "README.md"} {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("..", "..", rel))
			if err != nil {
				t.Fatal(err)
			}
			// Key rows by table, so the same target legitimately
			// appearing in two different tables is not flagged.
			type key struct{ table, target string }
			seen := map[key]int{}
			table := 0
			inTable := false
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "|") {
					inTable = false
					continue
				}
				if !inTable {
					table++
					inTable = true
				}
				if strings.HasPrefix(trimmed, "|--") || strings.Contains(trimmed, "|:--") {
					continue
				}
				m := docTableRow.FindStringSubmatch(trimmed)
				if m == nil {
					continue
				}
				name := strings.TrimSpace(m[1])
				if name == "" || name == "Target" || name == "Tool" {
					continue
				}
				k := key{strings.Repeat("t", table), name}
				seen[k]++
				if seen[k] > 1 {
					t.Errorf("%s: %q appears %d times in the same table; "+
						"collapse the duplicate rows, choosing each cell against "+
						"the adapter source rather than by position (#597)",
						rel, name, seen[k])
				}
			}
		})
	}
}
