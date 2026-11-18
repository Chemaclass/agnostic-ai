package conversations

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BenchmarkSearchPages times one page of a search that every session
// matches, over sessions started 9 hours apart, about 6 weeks in all:
//
//   - cold: a new scanner, which reads every store with no cache.
//   - first-page: the first page, which reads every store again.
//   - later-page: a page after it, asked while nothing changed.
//
// Run it with: go test -run '^$' -bench SearchPages ./internal/conversations
func BenchmarkSearchPages(b *testing.B) {
	scanner, files := benchClaudeStoreApart(b, 9*time.Hour)
	scanner.MarksPath = filepath.Join(b.TempDir(), "conversation-marks.json")
	for _, file := range files {
		// A chat the person read is no longer pinned to the first page.
		if err := scanner.Mark(ToolClaude, strings.TrimSuffix(filepath.Base(file), ".jsonl"), MarkSeen); err != nil {
			b.Fatal(err)
		}
	}
	const query = "safari"
	first := scanner.Scan(context.Background(), ScanOptions{Query: query})
	if first.Next == "" {
		b.Fatal("the search fits one page")
	}
	before, err := time.Parse(time.RFC3339, first.Next)
	if err != nil {
		b.Fatal(err)
	}
	later := ScanOptions{Query: query, Before: before}
	want := len(scanner.Scan(context.Background(), later).Conversations)
	if want == 0 {
		b.Fatal("the second page is empty")
	}

	b.Run("cold", func(b *testing.B) {
		for b.Loop() {
			fresh := &Scanner{Stores: scanner.Stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: scanner.Now, MarksPath: scanner.MarksPath}
			fresh.Scan(context.Background(), ScanOptions{Query: query})
		}
	})
	b.Run("first-page", func(b *testing.B) {
		for b.Loop() {
			scanner.Scan(context.Background(), ScanOptions{Query: query})
		}
	})
	b.Run("later-page", func(b *testing.B) {
		for b.Loop() {
			if got := len(scanner.Scan(context.Background(), later).Conversations); got != want {
				b.Fatalf("the second page has %d rows, want %d", got, want)
			}
		}
	})
}
