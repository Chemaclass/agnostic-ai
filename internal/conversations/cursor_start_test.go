package conversations

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cursorChat  = "c0c0c0c0-c0c0-4c0c-8c0c-c0c0c0c0c0c0"
	cursorOther = "d1d1d1d1-d1d1-4d1d-8d1d-d1d1d1d1d1d1"
	cursorNode  = "/Users/me/.local/share/cursor-agent/versions/2026.11.11-b912975/node"
)

// cursorAgentChain is agnostic-ai run by cursor-agent's shell, in a
// Ghostty tab; cursor-agent is pid 98.
func cursorAgentChain() []ProcessInfo {
	table := chain("agnostic-ai", "/bin/zsh", cursorNode, "/bin/zsh", "/usr/bin/login", "/Applications/Ghostty.app/Contents/MacOS/ghostty")
	table[2].TTY, table[3].TTY, table[4].TTY = "/dev/ttys005", "/dev/ttys005", "/dev/ttys005"
	return table
}

func writeCursorChatStores(t *testing.T, stores Stores, ids ...string) []string {
	t.Helper()
	var paths []string
	for _, id := range ids {
		path := filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), id, "store.db")
		write(t, path, "")
		paths = append(paths, path)
	}
	return paths
}

func TestCurrentSession_NamesTheChatACursorAgentRuns(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	dbs := writeCursorChatStores(t, stores, cursorChat, cursorOther)
	table := chain("agnostic-ai", "/bin/zsh", cursorNode, "/bin/zsh", "claude", "/bin/zsh")
	writeLiveSessionFile(t, stores, "96", started, idParent)
	variable := env(map[string]string{cursorChatVariable: cursorChat})

	for name, tc := range map[string]struct {
		held   map[string]int
		getenv func(string) string
	}{
		"variable and the chat it holds": {map[string]int{dbs[0]: 98, dbs[1]: 4242}, variable},
		"variable alone":                 {nil, variable},
		"the one chat it holds":          {map[string]int{dbs[0]: 98, dbs[1]: 4242}, env(nil)},
	} {
		ref, pid, err := CurrentSession(fakeProcesses{table: table, held: tc.held}, stores, tc.getenv, 100)
		if err != nil || ref != (Ref{Tool: ToolCursor, ID: cursorChat}) || pid != 98 {
			t.Errorf("%s: ref = %+v pid %d err %v", name, ref, pid, err)
		}
	}
}

func TestCurrentSession_RefusesACursorChatItCannotName(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	dbs := writeCursorChatStores(t, stores, cursorChat, cursorOther)
	table := chain("agnostic-ai", "/bin/zsh", cursorNode, "/bin/zsh", cursorNode)
	for name, tc := range map[string]struct {
		held   map[string]int
		getenv func(string) string
		want   string
	}{
		"an outer cursor-agent's variable": {map[string]int{dbs[0]: 98, dbs[1]: 96}, env(map[string]string{cursorChatVariable: cursorOther}), "CURSOR_CONVERSATION_ID names another chat than the one Cursor process 98 holds"},
		"several chats, no variable":       {map[string]int{dbs[0]: 98, dbs[1]: 98}, env(nil), "holds 2 chats, and CURSOR_CONVERSATION_ID is not set"},
		"no chat, no variable":             {nil, env(nil), "its process 98 holds no chat"},
		"a variable that is no chat id":    {nil, env(map[string]string{cursorChatVariable: "../x"}), "its process 98 holds no chat"},
	} {
		ref, _, err := CurrentSession(fakeProcesses{table: table, held: tc.held}, stores, tc.getenv, 100)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: ref %+v err %v, want %q", name, ref, err, tc.want)
		}
	}
}

func TestCurrentSession_RefusesACursorEditorChat(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	helper := "/Applications/Cursor.app/Contents/Frameworks/Cursor Helper (Plugin).app/Contents/MacOS/Cursor Helper (Plugin)"

	_, _, err := CurrentSession(fakeProcesses{table: chain("agnostic-ai", "/bin/zsh", helper, "/Applications/Cursor.app/Contents/MacOS/Cursor")}, stores, env(map[string]string{cursorChatVariable: cursorChat}), 100)

	if err == nil || !strings.Contains(err.Error(), "a Cursor editor chat cannot start a linked conversation") {
		t.Errorf("err = %v", err)
	}
}

func TestStart_FromCursorAgentLinksTheNewChatToIt(t *testing.T) {
	for _, tool := range []Tool{"", ToolCursor, ToolClaude, ToolCodex} {
		start := newFakeStart(t, cursorAgentChain(), "cursor-agent", "claude", "codex")
		if err := os.Remove(filepath.Join(start.stores.ClaudeDir, "sessions", "98.json")); err != nil {
			t.Fatal(err)
		}
		start.starter.Getenv = env(map[string]string{cursorChatVariable: cursorChat})

		reply, err := start.starter.Start(context.Background(), request(tool, "Write the notes"))

		if err != nil || len(start.launcher.calls) != 1 || !tabLine.MatchString(start.launcher.calls[0]) {
			t.Fatalf("--tool %q: calls %q err %v", tool, start.launcher.calls, err)
		}
		want := tool
		if want == "" {
			want = ToolCursor
		}
		id := tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
		if brief := readBrief(t, filepath.Join(start.starter.BriefsDir, id+".json")); brief.Tool != want || brief.Prompt != "Continues cursor:"+cursorChat+"\n\nWrite the notes" {
			t.Errorf("--tool %q: brief = %+v", tool, brief)
		}
		if reply.Parent != (Ref{Tool: ToolCursor, ID: cursorChat}) || reply.Tool != want || !strings.HasPrefix(reply.Message, "Started "+toolLabels[want]+" in a new Ghostty tab") {
			t.Errorf("--tool %q: reply = %+v", tool, reply)
		}
	}
}

func TestStart_StartsCursorAgentFromClaudeCodeAndCodex(t *testing.T) {
	fromCodex := chain("agnostic-ai", "/bin/zsh", "/opt/bin/codex")
	for name, tc := range map[string]struct {
		table  []ProcessInfo
		getenv func(string) string
		parent Ref
	}{
		"Claude Code": {ghosttyChain(), env(nil), Ref{Tool: ToolClaude, ID: idParent}},
		"Codex":       {fromCodex, env(map[string]string{"CODEX_THREAD_ID": idChild, "TERM_PROGRAM": "ghostty"}), Ref{Tool: ToolCodex, ID: idChild}},
	} {
		start := newFakeStart(t, tc.table, "cursor-agent")
		start.starter.Getenv = tc.getenv
		if tc.parent.Tool == ToolCodex {
			if err := os.Remove(filepath.Join(start.stores.ClaudeDir, "sessions", "98.json")); err != nil {
				t.Fatal(err)
			}
		}

		reply, err := start.starter.Start(context.Background(), request(ToolCursor, "Write the notes"))

		if err != nil || len(start.launcher.calls) != 1 || !tabLine.MatchString(start.launcher.calls[0]) {
			t.Fatalf("%s: calls %q err %v", name, start.launcher.calls, err)
		}
		id := tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
		dir := start.starter.BriefsDir
		if brief := readBrief(t, filepath.Join(dir, id+".json")); brief.Tool != ToolCursor || brief.Prompt != linkLine(tc.parent)+"\n\nWrite the notes" {
			t.Errorf("%s: brief = %+v", name, brief)
		}
		if reply.Parent != tc.parent || reply.Tool != ToolCursor || !strings.HasPrefix(reply.Message, "Started Cursor in a new Ghostty tab") {
			t.Errorf("%s: reply = %+v", name, reply)
		}
		var ran fakeExec
		if err := runner(t, dir, &ran).Run(id); err != nil || ran.path != "/opt/bin/cursor-agent" || len(ran.argv) != 2 || ran.argv[0] != "cursor-agent" || ran.argv[1] != linkLine(tc.parent)+"\n\nWrite the notes" {
			t.Errorf("%s: exec %q %q err %v", name, ran.path, ran.argv, err)
		}
	}
}

func TestStart_SaysWhenCursorAgentIsNotInstalled(t *testing.T) {
	start := newFakeStart(t, ghosttyChain(), "claude")

	_, err := start.starter.Start(context.Background(), request(ToolCursor, "Write the notes"))

	if err == nil || !strings.Contains(err.Error(), "Cursor is not installed: no cursor-agent command on PATH") || len(start.launcher.calls) != 0 {
		t.Errorf("err %v calls %q", err, start.launcher.calls)
	}
}

func TestScan_ReadsTheParentOfACursorAgentChatFromItsFirstPrompt(t *testing.T) {
	meta := hex.EncodeToString([]byte(`{"agentId":"` + cursorChat + `","name":"New Agent","createdAt":1762500000000}`))
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[]`,
		"store.db|FROM meta":               `[{"value":"` + meta + `"}]`,
	}), fakeProcesses{})
	write(t, stores.CursorDB, "")
	writeCursorChatStores(t, stores, cursorChat)
	write(t, filepath.Join(stores.CursorDir, "projects", "p1", "agent-transcripts", cursorChat, cursorChat+".jsonl"), lines(
		obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<timestamp>Wed Nov 12 2026</timestamp>\n<user_query>\nContinues claude:" + idParent + "\n\nWrite the release notes\n</user_query>"}}}},
	))
	writeSession(t, stores, idParent, typedPrompt("Plan the release"))
	scanner.Now = func() time.Time { return time.UnixMilli(1762800000000) }

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), cursorChat)

	if c.Parent == nil || *c.Parent != (Ref{Tool: ToolClaude, ID: idParent}) || c.Title != "Write the release notes" {
		t.Errorf("parent %+v title %q", c.Parent, c.Title)
	}
}

func TestScan_ListsCursorAgentChatsWithoutTheEditorIndex(t *testing.T) {
	for name, tc := range map[string]struct {
		breakIndex func(*Scanner)
		note       string
	}{
		"no index": {func(s *Scanner) { _ = os.Remove(s.Stores.CursorDB) }, "No Cursor chat index at "},
		"an index without chat headers": {func(s *Scanner) {
			query := s.SQLite
			s.SQLite = func(ctx context.Context, path, sql string) ([]json.RawMessage, error) {
				if path == s.Stores.CursorDB {
					return nil, errors.New("no such table: composerHeaders")
				}
				return query(ctx, path, sql)
			}
		}, "Could not read Cursor's chat index"},
	} {
		scanner, _ := cursorAgentScanner(t, fakeProcesses{}, cursorChat)
		tc.breakIndex(scanner)

		view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})

		var source Source
		for _, s := range view.Tools {
			if s.Tool == ToolCursor {
				source = s
			}
		}
		if !source.Found || !strings.HasPrefix(source.Note, tc.note) || len(view.Conversations) != 1 || view.Conversations[0].ID != cursorChat {
			t.Errorf("%s: source %+v rows %d", name, source, len(view.Conversations))
		}
	}
}
