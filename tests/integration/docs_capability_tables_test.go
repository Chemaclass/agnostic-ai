package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var docTableRow = regexp.MustCompile(`^\|\s*(?:\*\*)?([A-Za-z][A-Za-z0-9 ()/.-]*?)(?:\*\*)?\s*\|`)

func TestDocs_CapabilityTablesHaveNoDuplicateRows(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"docs/site/content/docs/target-behavior.md", "docs/site/content/docs/global-configuration.md", "README.md"} {
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
