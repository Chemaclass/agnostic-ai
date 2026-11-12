package conversations

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_TakesTheFirstPromptFromWhatThePersonTyped(t *testing.T) {
	notification := "<task-notification>\n<summary>Background command completed</summary>\n</task-notification>"
	cases := []struct {
		name  string
		first obj
		want  string
	}{
		{"a background task notification is skipped",
			obj{"type": "user", "origin": obj{"kind": "task-notification"}, "message": obj{"role": "user", "content": notification}},
			"Fix the login bug"},
		{"a line without origin from an older version counts",
			obj{"type": "user", "message": obj{"role": "user", "content": "Explain the cache"}},
			"Explain the cache"},
		{"a line the person wrote counts",
			obj{"type": "user", "origin": obj{"kind": "human"}, "message": obj{"role": "user", "content": "Explain the cache"}},
			"Explain the cache"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
			write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
				tc.first,
				obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z",
					"message": obj{"role": "user", "content": "Fix the login bug"}},
			))

			c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)

			if c.Title != tc.want || strings.Join(c.Opening, "|") != tc.want {
				t.Errorf("title %q opening %q, want %q", c.Title, c.Opening, tc.want)
			}
		})
	}
}
