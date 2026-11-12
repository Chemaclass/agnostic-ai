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

// benchCursorAgentStores writes that many cursor-agent chats, one
// written each hour, each with a few prompts and replies.
func benchCursorAgentStores(b *testing.B) Stores {
	b.Helper()
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
	return stores
}

// benchCursorAgentSQLite answers the metadata query of every chat store.
func benchCursorAgentSQLite(_ context.Context, path, query string) ([]json.RawMessage, error) {
	if filepath.Base(path) != "store.db" {
		return nil, nil
	}
	id := filepath.Base(filepath.Dir(path))
	meta := hex.EncodeToString([]byte(`{"agentId":"` + id + `","name":"New Agent","createdAt":1762500000000}`))
	return []json.RawMessage{json.RawMessage(`{"value":"` + meta + `"}`)}, nil
}

// BenchmarkScan_CursorAgentChatsCold times a first scan of that many
// cursor-agent chats, so every transcript is read.
func BenchmarkScan_CursorAgentChatsCold(b *testing.B) {
	stores := benchCursorAgentStores(b)
	for b.Loop() {
		scanner := &Scanner{Stores: stores, SQLite: benchCursorAgentSQLite, Processes: fakeProcesses{}, Now: time.Now}
		view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})
		if len(view.Tools) < 3 || view.Tools[2].Count != benchCursorAgentChats {
			b.Fatalf("Cursor source = %+v", view.Tools)
		}
	}
}

// cursorFilesBeforeTranscripts is cursorFiles before it named the
// transcripts of running chats: the status quo.
func (s *Scanner) cursorFilesBeforeTranscripts() []string {
	globalStorage := filepath.Dir(s.Stores.CursorDB)
	workspaces, _ := filepath.Glob(filepath.Join(filepath.Dir(globalStorage), "workspaceStorage", "*", "state.vscdb"))
	agents, _ := filepath.Glob(filepath.Join(s.Stores.CursorDir, "chats", "*", "*", "store.db"))
	files := withWAL(s.Stores.CursorDB)
	files = append(files, filepath.Join(globalStorage, "storage.json"))
	files = append(files, withWAL(workspaces...)...)
	return append(files, withWAL(agents...)...)
}

// BenchmarkStamp_CursorAgentChats times the Cursor part of a Stamp over
// that many chats, two of them running: before it named transcripts
// (status quo), with the running chats' transcripts (in use), and with
// every transcript found by a glob (every chat).
func BenchmarkStamp_CursorAgentChats(b *testing.B) {
	stores := benchCursorAgentStores(b)
	held := map[string]int{}
	for n := range 2 {
		id := fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", n)
		held[filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), id, "store.db")] = 4242
	}
	scanner := &Scanner{Stores: stores, SQLite: benchCursorAgentSQLite, Processes: fakeProcesses{held: held}, Now: time.Now}
	scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})
	if len(scanner.cursorLiveTranscripts) != 2 {
		b.Fatalf("running transcripts = %v", scanner.cursorLiveTranscripts)
	}
	for _, subject := range []struct {
		name  string
		files func() []string
	}{
		{"status_quo", scanner.cursorFilesBeforeTranscripts},
		{"running_transcripts", scanner.cursorFiles},
		{"every_transcript", func() []string {
			transcripts, _ := filepath.Glob(filepath.Join(stores.CursorDir, "projects", "*", "agent-transcripts", "*", "*.jsonl"))
			return append(scanner.cursorFilesBeforeTranscripts(), transcripts...)
		}},
	} {
		b.Run(subject.name, func(b *testing.B) {
			for b.Loop() {
				fileStamp(subject.files()...)
			}
		})
	}
}
