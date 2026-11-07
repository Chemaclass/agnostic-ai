package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// benchSessions is about the number of Claude Code sessions a regular
// user keeps; benchPrompt is a prompt of a few paragraphs.
const benchSessions = 150

var benchPrompt = strings.Repeat("Fix the login bug on Safari: the session cookie is lost after the redirect. Check middleware.go and add a test.\n", 3)

func benchTexts() []texts {
	list := make([]texts, benchSessions)
	for i := range list {
		list[i] = texts{title: fmt.Sprintf("Session %d", i), firstPrompt: benchPrompt + fmt.Sprint(i), lastPrompt: fmt.Sprint(i) + benchPrompt}
	}
	return list
}

// BenchmarkShownTexts compares checking every row's texts on each scan
// (direct, what each scan did before) with reusing the previous scan's
// result (cached), the cost of a warm refresh where nothing changed.
func BenchmarkShownTexts(b *testing.B) {
	list := benchTexts()
	b.Run("direct", func(b *testing.B) {
		for b.Loop() {
			for _, t := range list {
				showTexts(t)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		var cache shownCache
		cache.start()
		for _, t := range list {
			cache.get(t)
		}
		cache.finish(context.Background())
		for b.Loop() {
			cache.start()
			for _, t := range list {
				cache.get(t)
			}
			cache.finish(context.Background())
		}
	})
}

// BenchmarkScan_Warm times a refresh of the first page when no session
// changed: the request the Conversations page sends on every visit.
func BenchmarkScan_Warm(b *testing.B) {
	home := b.TempDir()
	stores := Stores{Home: home, ClaudeDir: filepath.Join(home, ".claude"), CodexDir: filepath.Join(home, ".codex"), CursorDB: filepath.Join(home, "Cursor", "state.vscdb"), CursorDir: filepath.Join(home, ".cursor")}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	project := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		b.Fatal(err)
	}
	for i, t := range benchTexts() {
		id := fmt.Sprintf("%08x-1111-4111-8111-111111111111", i)
		var lines []string
		for _, line := range []map[string]any{
			{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339), "message": map[string]any{"role": "user", "content": t.firstPrompt}},
			{"type": "last-prompt", "lastPrompt": t.lastPrompt},
		} {
			data, _ := json.Marshal(line)
			lines = append(lines, string(data))
		}
		if err := os.WriteFile(filepath.Join(project, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	scanner := &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: func() time.Time { return now }}
	scanner.Scan(context.Background(), ScanOptions{})
	for b.Loop() {
		scanner.Scan(context.Background(), ScanOptions{})
	}
}
