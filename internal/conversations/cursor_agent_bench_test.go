package conversations

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// benchCursorAgentChats is a heavy cursor-agent user's chat count.
const benchCursorAgentChats = 2000

// BenchmarkScan_CursorAgentChatsCold times a first scan of that many
// cursor-agent chats, one written each hour, each with a few prompts and
// replies, so every transcript is read.
func BenchmarkScan_CursorAgentChatsCold(b *testing.B) {
	dir := b.TempDir()
	cursorDir := filepath.Join(dir, "cursor")
	stores := Stores{Home: dir, ClaudeDir: filepath.Join(dir, "claude"), CodexDir: filepath.Join(dir, "codex"), CursorDB: filepath.Join(dir, "Cursor", "state.vscdb"), CursorDir: cursorDir}
	put := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	put(stores.CursorDB, "")
	line := func(role, text string) string {
		data, _ := json.Marshal(map[string]any{"role": role, "message": map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}})
		return string(data) + "\n"
	}
	for n := range benchCursorAgentChats {
		id := fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", n)
		store := filepath.Join(cursorDir, "chats", md5Hex("/tmp/app"), id, "store.db")
		put(store, "")
		// One chat an hour: the first page holds the past week's 168.
		written := time.Now().Add(-time.Duration(n) * time.Hour)
		if err := os.Chtimes(store, written, written); err != nil {
			b.Fatal(err)
		}
		var transcript strings.Builder
		for turn := range 4 {
			transcript.WriteString(line("user", fmt.Sprintf("<user_query>%s %d %d</user_query>", benchPrompt, n, turn)))
			transcript.WriteString(line("assistant", fmt.Sprintf("%d %d %s", n, turn, benchReply)))
		}
		put(filepath.Join(cursorDir, "projects", "p1", "agent-transcripts", id, id+".jsonl"), transcript.String())
	}
	sqlite := func(_ context.Context, path, query string) ([]json.RawMessage, error) {
		if filepath.Base(path) != "store.db" {
			return nil, nil
		}
		id := filepath.Base(filepath.Dir(path))
		meta := hex.EncodeToString([]byte(`{"agentId":"` + id + `","name":"New Agent","createdAt":1762500000000}`))
		return []json.RawMessage{json.RawMessage(`{"value":"` + meta + `"}`)}, nil
	}
	for b.Loop() {
		scanner := &Scanner{Stores: stores, SQLite: sqlite, Processes: fakeProcesses{}, Now: time.Now}
		view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})
		if len(view.Tools) < 3 || view.Tools[2].Count != benchCursorAgentChats {
			b.Fatalf("Cursor source = %+v", view.Tools)
		}
	}
}
