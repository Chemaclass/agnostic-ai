package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func writeSession(t *testing.T, stores Stores, id string, values ...any) string {
	t.Helper()
	file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl")
	write(t, file, lines(values...))
	return file
}

func typedPrompt(text string) obj {
	return obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z", "message": obj{"role": "user", "content": text}}
}

func agentLine(blocks ...obj) obj {
	return obj{"type": "assistant", "message": obj{"role": "assistant", "content": blocks}}
}

func textPart(text string) obj { return obj{"type": "text", "text": text} }

func askPart(question string) obj {
	return obj{"type": "tool_use", "name": "AskUserQuestion", "input": obj{"questions": []obj{{"question": question, "header": "Choice", "options": []obj{{"label": "A"}, {"label": "B"}}, "multiSelect": false}}}}
}

func toolResultLine(text string) obj {
	return obj{"type": "user", "message": obj{"role": "user", "content": []obj{{"type": "tool_result", "content": text}}}}
}

func replyOfSession(t *testing.T, values ...any) Conversation {
	t.Helper()
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeSession(t, stores, idSafari, values...)
	return byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
}

func TestScan_ShowsTheStartOfTheAgentsLatestMessage(t *testing.T) {
	c := replyOfSession(t,
		typedPrompt("Fix the login bug"),
		obj{"type": "last-prompt", "lastPrompt": "Fix the login bug"},
		agentLine(obj{"type": "thinking", "thinking": "Look at the cookie first."}),
		agentLine(textPart("I fixed the cookie.\n\nThe test passes now.")),
	)

	if c.Reply != "I fixed the cookie. The test passes now." {
		t.Errorf("reply = %q", c.Reply)
	}
	if actual := asJSON(t, c); !strings.Contains(actual, `"prompt": "Fix the login bug",
  "reply": "I fixed the cookie. The test passes now.",`) {
		t.Errorf("the reply must follow the prompt in the JSON:\n%s", actual)
	}
}

func TestScan_ShowsTheQuestionAnAgentAsks(t *testing.T) {
	asked := []any{typedPrompt("Add a database"), agentLine(textPart("Two databases fit.")), agentLine(askPart("Which database should the app use?"))}
	if c := replyOfSession(t, asked...); c.Reply != "Which database should the app use?" {
		t.Errorf("an open question = %q", c.Reply)
	}
	if c := replyOfSession(t, append(asked, toolResultLine("Postgres"))...); c.Reply != "Two databases fit." {
		t.Errorf("an answered question must leave the message, actual %q", c.Reply)
	}
	for message, want := range map[string]string{
		"I can split the parser.\n\nShould I also rename the package?":     "Should I also rename the package?",
		"I can split the parser.\n\n**Should I also rename the package?**": "Should I also rename the package?",
		"Is it ready? Yes, I ran the suite.":                               "Is it ready? Yes, I ran the suite.",
	} {
		if c := replyOfSession(t, typedPrompt("Split the parser"), agentLine(textPart(message))); c.Reply != want {
			t.Errorf("message %q: reply = %q, want %q", message, c.Reply, want)
		}
	}
}

func TestShowTexts_CutsALongReplyWithAnEllipsis(t *testing.T) {
	long := "## Summary\n\n- I changed **the cookie** in `middleware.go`.\n" + strings.Repeat("Then I ran the whole suite again. ", 150)

	shown := showTexts(texts{reply: long})

	if utf8.RuneCountInString(shown.reply) != replyRunes || !strings.HasSuffix(shown.reply, "…") {
		t.Errorf("reply has %d runes: %q", utf8.RuneCountInString(shown.reply), shown.reply)
	}
	if !strings.HasPrefix(shown.reply, "Summary I changed the cookie in middleware.go. Then") {
		t.Errorf("markup must not show, actual %q", shown.reply)
	}
	reply, _ := agentSaid(long, long, "")
	if len(reply) > replyCheckedBytes || len(reply) < replyCheckedBytes-40 || !strings.HasPrefix(long, reply) || long[len(reply)] != ' ' {
		t.Errorf("a reader keeps whole words up to %d bytes, actual %d bytes: %q", replyCheckedBytes, len(reply), reply)
	}
}

func TestFill_WithholdsAReplyThatMayHoldACredential(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeSession(t, stores, idSafari, typedPrompt("Deploy the app"), agentLine(textPart("Use password hunter2 for the staging login.")))
	writeSession(t, stores, idCache, typedPrompt("Use password hunter2 for the staging login"), agentLine(textPart("I logged in with password hunter2.")))
	writeSession(t, stores, idSecret, typedPrompt("Pick a login"), agentLine(textPart("Two logins fit.")), agentLine(askPart("Can I use password hunter2 for staging?")))

	for range 2 {
		view := scanner.Scan(context.Background(), ScanOptions{})
		if c := byID(t, view, idSafari); c.Reply != "" || c.Title != "Deploy the app" || strings.Join(c.Withheld, ",") != "reply" {
			t.Errorf("a reply naming a password: reply %q title %q withheld %v", c.Reply, c.Title, c.Withheld)
		}
		if c := byID(t, view, idCache); c.Reply != "" || strings.Join(c.Withheld, ",") != "title,opening,reply" {
			t.Errorf("withheld must not grow from one scan to the next: reply %q withheld %v", c.Reply, c.Withheld)
		}
		if c := byID(t, view, idSecret); c.Reply != "" || strings.Join(c.Withheld, ",") != "reply" {
			t.Errorf("a question naming a password: reply %q withheld %v", c.Reply, c.Withheld)
		}
	}
}

func TestScan_ShowsNoReplyBeforeTheAgentAnswers(t *testing.T) {
	if c := replyOfSession(t, typedPrompt("Fix the login bug")); c.Reply != "" {
		t.Errorf("no agent message yet, reply = %q", c.Reply)
	}
	if c := replyOfSession(t, typedPrompt("Fix the login bug"), agentLine(textPart("Done.")), typedPrompt("Now update the docs")); c.Reply != "" {
		t.Errorf("a reply older than the latest prompt must not show, actual %q", c.Reply)
	}
}

func TestScan_KeepsToolCallsResultsAndSubagentOutputOutOfTheReply(t *testing.T) {
	// A subagent line at the start marks a subagent's own session, so the
	// large result keeps it out of the head.
	c := replyOfSession(t,
		typedPrompt("Fix the login bug"),
		toolResultLine(strings.Repeat("x", 3*claudeWindow)),
		agentLine(textPart("The real answer.")),
		agentLine(obj{"type": "tool_use", "name": "Bash", "input": obj{"command": "go test ./..."}}),
		toolResultLine("ok all tests passed"),
		obj{"type": "user", "message": obj{"role": "user", "content": []obj{{"type": "tool_result", "content": []obj{textPart("result as text")}}}}},
		obj{"type": "assistant", "isSidechain": true, "message": obj{"role": "assistant", "content": []obj{textPart("A subagent says this.")}}},
		obj{"type": "api-request-blob", "message": obj{"role": "assistant", "content": []obj{textPart("A request blob.")}}},
		obj{"type": "assistant", "isApiErrorMessage": true, "message": obj{"role": "assistant", "content": []obj{textPart("API Error: overloaded")}}},
		obj{"type": "assistant", "message": obj{"role": "assistant", "model": "<synthetic>", "content": []obj{textPart("No response requested.")}}},
	)

	if c.Reply != "The real answer." {
		t.Errorf("reply = %q", c.Reply)
	}
}

func TestScan_ReadsAReplyThatSitsBehindALargeLine(t *testing.T) {
	behind := func(size int) Conversation {
		return replyOfSession(t,
			typedPrompt("Read the logs"),
			toolResultLine(strings.Repeat("x", 3*claudeWindow)),
			agentLine(textPart("Found it.")),
			toolResultLine(strings.Repeat("y", size)),
		)
	}
	if c := behind(100 << 10); c.Reply != "Found it." {
		t.Errorf("a message behind a 100 KB line = %q", c.Reply)
	}
	if c := behind(300 << 10); c.Reply != "" || c.Title != "Read the logs" {
		t.Errorf("a message out of reach shows no line, actual reply %q title %q", c.Reply, c.Title)
	}
}

func TestScan_ReadsCodexRepliesFromTheLatestTurn(t *testing.T) {
	answered, asking, working := "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"
	threads := `[
		{"id":"` + answered + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762520000000,"originator":"codex-tui","source":"cli","title":"Fix the parser","first_user_message":"Fix the parser"},
		{"id":"` + asking + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex-tui","source":"cli","title":"Pick a queue","first_user_message":"Pick a queue"},
		{"id":"` + working + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762500000000,"originator":"codex-tui","source":"cli","title":"Write the docs","first_user_message":"Write the docs"}
	]`
	prompts := `[{"thread_id":"` + working + `","content":"[{\"type\":\"text\",\"text\":\"Now the API page\"}]"}]`
	replies := `[
		{"thread_id":"` + answered + `","start":"Done, the tests pass.","ending":"Done, the tests pass.","length":21,"asked":""},
		{"thread_id":"` + asking + `","start":"Two queues fit.","ending":"Two queues fit.","length":15,"asked":"Which queue should the jobs use?"}
	]`
	scan := func(answers map[string]string) View {
		scanner, stores := testScanner(t, fakeStores(t, answers), fakeProcesses{})
		write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
		write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")
		return scanner.Scan(context.Background(), ScanOptions{})
	}

	view := scan(map[string]string{"state_5.sqlite|FROM threads": threads, "thread_history_1.sqlite|$.content": prompts, "thread_history_1.sqlite|$.questions": replies})

	for id, want := range map[string]string{answered: "Done, the tests pass.", asking: "Which queue should the jobs use?", working: ""} {
		if c := byID(t, view, id); c.Reply != want {
			t.Errorf("thread %s: reply = %q, want %q", id[:4], c.Reply, want)
		}
	}
	failed := scan(map[string]string{"state_5.sqlite|FROM threads": threads, "thread_history_1.sqlite|$.content": prompts})
	if c := byID(t, failed, working); c.Reply != "" || c.Prompt != "Now the API page" || len(failed.Conversations) != 3 {
		t.Errorf("a failed reply query must leave the rows and prompts: %s", asJSON(t, failed.Conversations))
	}
	if note := failed.Tools[1].Note; note != "Could not read Codex's latest replies from thread_history_1.sqlite" {
		t.Errorf("a failed reply query must say so, note = %q", note)
	}
}

func TestScan_AsksForCodexRepliesInBatches(t *testing.T) {
	var threads []string
	for i := range 1200 {
		threads = append(threads, fmt.Sprintf(`{"id":"%08x-0000-4000-8000-000000000000","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762500000000,"originator":"codex-tui","source":"cli","title":"Thread","first_user_message":"Start"}`, i))
	}
	state := fakeStores(t, map[string]string{"state_5.sqlite|FROM threads": "[" + strings.Join(threads, ",") + "]"})
	var batches []int
	scanner, stores := testScanner(t, func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if filepath.Base(path) == "state_5.sqlite" {
			return state(ctx, path, query)
		}
		if strings.Contains(query, "$.questions") {
			batches = append(batches, strings.Count(query, "-0000-4000-8000-"))
			return []json.RawMessage{json.RawMessage(`{"thread_id":"00000000-0000-4000-8000-000000000000","start":"Done.","ending":"Done.","length":5,"asked":""}`)}, nil
		}
		return nil, nil
	}, fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")

	view := scanner.Scan(context.Background(), ScanOptions{Query: "thread"})

	if fmt.Sprint(batches) != "[500 500 200]" {
		t.Errorf("batches of ids = %v", batches)
	}
	if c := byID(t, view, "00000000-0000-4000-8000-000000000000"); c.Reply != "Done." {
		t.Errorf("reply = %q", c.Reply)
	}
}

func TestScan_ShowsNoReplyForCloudRows(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return []Conversation{cloudRow("cse_a1", "Fix the parser", "2026-11-07T11:30:00Z")}, nil
		},
	}}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if len(view.Conversations) == 0 {
		t.Fatalf("no rows: %s", asJSON(t, view))
	}
	for _, c := range view.Conversations {
		if c.Reply != "" || slices.Contains(c.Withheld, FieldReply) {
			t.Errorf("%s row %s must have no reply line: %s", c.Tool, c.ID, asJSON(t, c))
		}
	}
}

// cursorBubble is one message of a Cursor editor chat: its header and
// its bubble. A message with no text, such as a tool call, has a
// header that says so.
type cursorBubble struct {
	kind int
	text string
}

func cursorUser(text string) cursorBubble  { return cursorBubble{kind: 1, text: text} }
func cursorAgent(text string) cursorBubble { return cursorBubble{kind: 2, text: text} }

// cursorEditorStore writes a Cursor state database with the tables the
// chat queries read. Chats whose id starts with "c" keep their message
// headers in composerConvHeaders chunks, the others inline.
func cursorEditorStore(t *testing.T, path string, recency int64, chats map[string][]cursorBubble) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	quote := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return "'" + strings.ReplaceAll(string(data), "'", "''") + "'"
	}
	sql := []string{
		`CREATE TABLE composerHeaders (composerId TEXT PRIMARY KEY, workspaceId TEXT, createdAt INTEGER, recency INTEGER, isSubagent INTEGER, isArchived INTEGER, value TEXT)`,
		`CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`,
	}
	for id, bubbles := range chats {
		sql = append(sql, fmt.Sprintf(`INSERT INTO composerHeaders VALUES ('%s', 'ws1', %d, %d, 0, 0, %s)`, id, recency, recency, quote(obj{"name": "Chat " + id[:2], "workspaceIdentifier": obj{"uri": obj{"fsPath": "/tmp/app"}}})))
		var headers []obj
		for i, bubble := range bubbles {
			bubbleID := fmt.Sprintf("b%d", i)
			header := obj{"bubbleId": bubbleID, "type": bubble.kind, "grouping": obj{"isRenderable": true}}
			if bubble.text != "" {
				header["grouping"] = obj{"isRenderable": true, "hasText": true}
			}
			headers = append(headers, header)
			sql = append(sql, fmt.Sprintf(`INSERT INTO cursorDiskKV VALUES ('bubbleId:%s:%s', %s)`, id, bubbleID, quote(obj{"_v": 3, "type": bubble.kind, "bubbleId": bubbleID, "text": bubble.text})))
		}
		if strings.HasPrefix(id, "c") {
			sql = append(sql, fmt.Sprintf(`INSERT INTO cursorDiskKV VALUES ('composerData:%s', %s)`, id, quote(obj{"_v": 10})))
			for start := 0; start < len(headers); start += 2 {
				sql = append(sql, fmt.Sprintf(`INSERT INTO cursorDiskKV VALUES ('composerConvHeaders:%s:%d', %s)`, id, start/2, quote(headers[start:min(start+2, len(headers))])))
			}
			continue
		}
		sql = append(sql, fmt.Sprintf(`INSERT INTO cursorDiskKV VALUES ('composerData:%s', %s)`, id, quote(obj{"_v": 10, "fullConversationHeadersOnly": headers})))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	if out, err := exec.Command("sqlite3", path, strings.Join(sql, ";\n")+";").CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v: %s", err, out)
	}
}

func TestScan_ReadsTheCursorEditorReplyThatFollowsTheLatestPrompt(t *testing.T) {
	answered, stale, asking, secret, long, chunked, silent := "a1111111", "s1111111", "q1111111", "k1111111", "l1111111", "c1111111", "n1111111"
	recency := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC).UnixMilli()
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	chosen := 0
	scanner.SQLite = func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if strings.Contains(query, "chosen(composerId)") {
			chosen++
		}
		return RunSQLite(ctx, path, query)
	}
	cursorEditorStore(t, stores.CursorDB, recency, map[string][]cursorBubble{
		answered: {cursorUser("Fix the login"), cursorAgent("I will look."), cursorAgent(""), cursorAgent("Done, the tests pass."), cursorAgent("")},
		stale:    {cursorUser("Fix the login"), cursorAgent("An answer to the first prompt."), cursorUser("Now the docs"), cursorAgent("")},
		asking:   {cursorUser("Split the parser"), cursorAgent("I can split the parser.\n\nShould I also rename the package?")},
		secret:   {cursorUser("Deploy the app"), cursorAgent("Use password hunter2 for the staging login.")},
		long:     {cursorUser("Migrate"), cursorAgent(strings.Repeat("The migration ran on every table. ", 200))},
		chunked:  {cursorUser("Fix the build"), cursorAgent("An old answer."), cursorUser("Run it again"), cursorAgent(""), cursorAgent("The webhook build passes now.")},
		silent:   {cursorUser("Wait for it")},
	})

	view := scanner.Scan(context.Background(), ScanOptions{})

	for id, want := range map[string]string{
		answered: "Done, the tests pass.",
		stale:    "",
		asking:   "Should I also rename the package?",
		secret:   "",
		chunked:  "The webhook build passes now.",
		silent:   "",
	} {
		if c := byID(t, view, id); c.Reply != want {
			t.Errorf("chat %s: reply = %q, want %q", id, c.Reply, want)
		}
	}
	if c := byID(t, view, secret); strings.Join(c.Withheld, ",") != FieldReply {
		t.Errorf("a reply naming a password must be withheld: %s", asJSON(t, c))
	}
	if c := byID(t, view, long); utf8.RuneCountInString(c.Reply) != replyRunes || !strings.HasSuffix(c.Reply, "…") {
		t.Errorf("a long reply is cut to %d runes, actual %q", replyRunes, c.Reply)
	}
	if c := byID(t, view, chunked); c.Prompt != "Run it again" {
		t.Errorf("chunked chat prompt = %q", c.Prompt)
	}
	if found := scanner.Scan(context.Background(), ScanOptions{Query: "webhook"}); found.Matches == nil || *found.Matches != 1 {
		t.Errorf("search must reach a Cursor reply: %s", asJSON(t, found))
	}
	if chosen != 1 {
		t.Errorf("an unchanged chat is read once, read %d times", chosen)
	}
}

func TestScan_ReadsTheCursorAgentReplyThatFollowsTheLatestPrompt(t *testing.T) {
	answered, stale, asking, secret, long := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	scanner, _ := cursorAgentScanner(t, fakeProcesses{}, answered, stale, asking, secret, long)
	prompt := func(text string) obj {
		return obj{"role": "user", "message": obj{"content": []obj{textPart("<timestamp>Friday</timestamp>\n<user_query>" + text + "</user_query>")}}}
	}
	said := func(parts ...obj) obj { return obj{"role": "assistant", "message": obj{"content": parts}} }
	tool := obj{"type": "tool_use", "name": "Shell", "input": obj{"command": "go test ./..."}}
	transcripts := map[string][]any{
		answered: {prompt("Fix the CI job"), said(textPart("I will look."), tool), said(textPart("The CI job passes now.")), said(tool)},
		stale:    {prompt("Fix the CI job"), said(textPart("Done.")), prompt("Now the docs"), said(tool)},
		asking:   {obj{"role": "user", "message": obj{"content": []obj{textPart("<user_info>os</user_info>")}}}, prompt("Split it"), said(textPart("I can split it.\n\nShould I also rename the package?"))},
		secret:   {prompt("Deploy"), said(textPart("Use password hunter2 for the staging login."))},
		long:     {prompt("Migrate"), said(textPart(strings.Repeat("The migration ran on every table. ", 200)))},
	}
	for id, values := range transcripts {
		write(t, filepath.Join(scanner.Stores.CursorDir, "projects", "p1", "agent-transcripts", id, id+".jsonl"), lines(values...))
	}

	view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})

	for id, want := range map[string]string{
		answered: "The CI job passes now.",
		stale:    "",
		asking:   "Should I also rename the package?",
		secret:   "",
	} {
		if c := byID(t, view, id); c.Reply != want {
			t.Errorf("chat %s: reply = %q, want %q", id[:1], c.Reply, want)
		}
	}
	if c := byID(t, view, secret); strings.Join(c.Withheld, ",") != FieldReply {
		t.Errorf("a reply naming a password must be withheld: %s", asJSON(t, c))
	}
	if c := byID(t, view, long); utf8.RuneCountInString(c.Reply) != replyRunes || !strings.HasSuffix(c.Reply, "…") {
		t.Errorf("a long reply is cut to %d runes, actual %q", replyRunes, c.Reply)
	}
}

func TestMatching_SearchesTheReply(t *testing.T) {
	list := []Conversation{
		{ID: "shown", Title: "Fix the login", Reply: "The session cookie is set again."},
		{ID: "hidden", Title: "Fix the login", Withheld: []string{FieldReply}},
	}

	kept := matching(list, queryWords("Cookie"))

	if len(kept) != 1 || kept[0].ID != "shown" {
		t.Errorf("matched %s", asJSON(t, kept))
	}
}

func TestScan_RereadsTheReplyOnlyWhenTheFileChanges(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	file := writeSession(t, stores, idSafari, typedPrompt("Fix the login bug"), agentLine(textPart("First answer.")))
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Reply != "First answer." {
		t.Fatalf("reply = %q", c.Reply)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	writeSession(t, stores, idSafari, typedPrompt("Fix the login bug"), agentLine(textPart("Other answer.")))
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Reply != "First answer." {
		t.Errorf("a file of the same size and time must not be read again, reply = %q", c.Reply)
	}

	writeSession(t, stores, idSafari, typedPrompt("Fix the login bug"), agentLine(textPart("A longer, second answer.")))
	if err := os.Chtimes(file, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Reply != "A longer, second answer." {
		t.Errorf("a changed file must be read again, reply = %q", c.Reply)
	}
}

func TestScan_KeepsTheChosenNameOverARowWithAReply(t *testing.T) {
	scanner, stores, _ := renamingScanner(t)
	writeSession(t, stores, idSafari, typedPrompt("Fix the login bug"), agentLine(textPart("Done.")))
	scanner.Scan(context.Background(), ScanOptions{})
	if err := scanner.Rename(ToolClaude, idSafari, "Safari login"); err != nil {
		t.Fatal(err)
	}

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)

	if c.Title != "Safari login" || c.Reply != "Done." {
		t.Errorf("title %q reply %q", c.Title, c.Reply)
	}
}

// codexHistory writes a history store with the tables and indexes the
// reply query relies on, and an item per row: thread, turn, ordinal,
// type and the item's JSON.
func codexHistory(t *testing.T, path string, items [][5]string) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	sql := []string{
		`CREATE TABLE thread_items (thread_id TEXT NOT NULL, turn_id TEXT NOT NULL, item_id TEXT NOT NULL, rollout_ordinal INTEGER NOT NULL, created_at_ms INTEGER NOT NULL, item_json TEXT NOT NULL, item_type TEXT NOT NULL DEFAULT '', PRIMARY KEY (thread_id, turn_id, item_id))`,
		`CREATE UNIQUE INDEX idx_thread_items_page ON thread_items(thread_id, rollout_ordinal)`,
		`CREATE INDEX idx_thread_items_user_messages ON thread_items(thread_id, rollout_ordinal) WHERE item_type = 'userMessage'`,
	}
	for i, item := range items {
		sql = append(sql, fmt.Sprintf(`INSERT INTO thread_items VALUES ('%s', '%s', 'item-%d', %s, 0, '%s', '%s')`, item[0], item[1], i, item[2], strings.ReplaceAll(item[4], "'", "''"), item[3]))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sqlite3", path, strings.Join(sql, ";\n")+";").CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v: %s", err, out)
	}
}

func codexAgentItem(text string, question string) string {
	item := obj{"type": "agentMessage", "text": text, "phase": "final_answer"}
	if question != "" {
		item["questions"] = []obj{{"title": question, "options": []obj{}}}
	}
	data, _ := json.Marshal(item)
	return string(data)
}

func TestScan_ReadsTheCodexReplyThatFollowsTheLatestPrompt(t *testing.T) {
	ids := []string{"11111111-0000-4000-8000-000000000001", "11111111-0000-4000-8000-000000000002", "11111111-0000-4000-8000-000000000003", "11111111-0000-4000-8000-000000000004", "11111111-0000-4000-8000-000000000005", "11111111-0000-4000-8000-000000000006", "11111111-0000-4000-8000-000000000007"}
	answered, typedDuringTurn, quietTurn, asking, long, unlisted, cutLine := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6]
	prompt := `{"type":"userMessage","content":[{"type":"text","text":"Go on"}]}`
	// The end slice of this message starts inside its last line, which
	// is too long to be a question and must not pass as one.
	cutLineText := "Done.\n" + strings.Repeat("x", 5000) + " password hunter2" + strings.Repeat(" ", 4088) + "?\n"
	longText := strings.Repeat("The migration ran on every table and the counts match. ", 400) + "\n\nShould I drop the old tables now?"
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	codexHistory(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), [][5]string{
		{answered, "t1", "1", "userMessage", prompt},
		{answered, "t1", "2", "agentMessage", codexAgentItem("Done, the tests pass.", "")},
		{typedDuringTurn, "t1", "1", "userMessage", prompt},
		{typedDuringTurn, "t1", "2", "agentMessage", codexAgentItem("An answer to the first prompt.", "")},
		{typedDuringTurn, "t1", "3", "userMessage", prompt},
		{typedDuringTurn, "t1", "4", "commandExecution", `{"type":"commandExecution"}`},
		{quietTurn, "t1", "1", "userMessage", prompt},
		{quietTurn, "t1", "2", "agentMessage", codexAgentItem("The earlier turn's answer.", "")},
		{quietTurn, "t2", "3", "reasoning", `{"type":"reasoning"}`},
		{asking, "t1", "1", "userMessage", prompt},
		{asking, "t1", "2", "agentMessage", codexAgentItem("Two queues fit.", "Which queue should the jobs use?")},
		{long, "t1", "1", "userMessage", prompt},
		{long, "t1", "2", "agentMessage", codexAgentItem(longText, "")},
		{cutLine, "t1", "1", "userMessage", prompt},
		{cutLine, "t1", "2", "agentMessage", codexAgentItem(cutLineText, "")},
		{unlisted, "t1", "1", "userMessage", prompt},
		{unlisted, "t1", "2", "agentMessage", codexAgentItem("Not listed.", "")},
	})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	var threads []string
	for _, id := range append(ids[:5:5], cutLine) {
		threads = append(threads, `{"id":"`+id+`","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762500000000,"originator":"codex-tui","source":"cli","title":"Thread","first_user_message":"Start"}`)
	}
	state := fakeStores(t, map[string]string{"state_5.sqlite|FROM threads": "[" + strings.Join(threads, ",") + "]"})
	var replyRows int
	scanner.SQLite = func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if filepath.Base(path) == "state_5.sqlite" {
			return state(ctx, path, query)
		}
		rows, err := RunSQLite(ctx, path, query)
		if strings.Contains(query, "$.questions") {
			replyRows = len(rows)
			for _, row := range rows {
				if len(row) > 3*replyCheckedBytes {
					t.Errorf("a reply row must be bounded, it has %d bytes", len(row))
				}
			}
		}
		return rows, err
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	for id, want := range map[string]string{
		answered:        "Done, the tests pass.",
		typedDuringTurn: "",
		quietTurn:       "The earlier turn's answer.",
		asking:          "Which queue should the jobs use?",
		long:            "Should I drop the old tables now?",
		cutLine:         "Done.",
	} {
		if c := byID(t, view, id); c.Reply != want {
			t.Errorf("thread %s: reply = %q, want %q", id[len(id)-1:], c.Reply, want)
		}
	}
	if replyRows > 6 {
		t.Errorf("the query must read only the listed threads, it returned %d rows", replyRows)
	}
}

func TestShowTexts_WithholdsATokenThatMarkupSplits(t *testing.T) {
	for _, reply := range []string{
		"The key is `ghp_`AbCd1234EfGh5678IjKl here.",
		"Use **sk-ant-**api03-AbCd1234EfGh5678 for it.",
	} {
		if shown := showTexts(texts{reply: reply}); shown.reply != "" || strings.Join(shown.withheld, ",") != FieldReply {
			t.Errorf("reply %q shows %q, withheld %v", reply, shown.reply, shown.withheld)
		}
		if shown := showTexts(texts{reply: "Two keys fit.", question: reply}); shown.reply != "" || strings.Join(shown.withheld, ",") != FieldReply {
			t.Errorf("question %q shows %q, withheld %v", reply, shown.reply, shown.withheld)
		}
	}
}

func TestAgentSaid_KeepsTheTextTheCheckReadsUpToItsCap(t *testing.T) {
	secret := strings.Repeat("The run is long and the log is longer. ", 60) + "Then use password hunter2 for the staging login."
	if len(secret) > replyCheckedBytes {
		t.Fatalf("the fixture must fit the cap, it has %d bytes", len(secret))
	}
	if shown := showTexts(texts{reply: secret}); shown.reply != "" {
		t.Errorf("a credential past the first kilobyte must withhold the reply, shown %q", shown.reply)
	}
}

func TestScan_KeepsTheReplyWhenABackgroundTaskReports(t *testing.T) {
	c := replyOfSession(t,
		typedPrompt("Run the suite in the background"),
		agentLine(textPart("The suite runs; I will report when it ends.")),
		obj{"type": "user", "origin": obj{"kind": "task-notification"}, "promptSource": "system", "message": obj{"role": "user", "content": "<task-notification>\n<task-id>b1</task-id>\n<status>completed</status>\n</task-notification>"}},
	)
	if c.Reply != "The suite runs; I will report when it ends." {
		t.Errorf("a task notification is no prompt, reply = %q", c.Reply)
	}
	asked := replyOfSession(t,
		typedPrompt("Pick a database"),
		agentLine(askPart("Which database should the app use?")),
		obj{"type": "user", "origin": obj{"kind": "task-notification"}, "message": obj{"role": "user", "content": "<task-notification>done</task-notification>"}},
	)
	if asked.Reply != "Which database should the app use?" {
		t.Errorf("a task notification does not answer a question, reply = %q", asked.Reply)
	}
	if c := replyOfSession(t, typedPrompt("Fix it"), agentLine(textPart("Done.")), obj{"type": "user", "origin": obj{"kind": "human"}, "message": obj{"role": "user", "content": "Now the docs"}}); c.Reply != "" {
		t.Errorf("a prompt the person typed still clears the reply, actual %q", c.Reply)
	}
}

func TestScan_ReadsNoQuestionFromALineTooLongToKeep(t *testing.T) {
	message := "Done.\n" + strings.Repeat("x", 5000) + " password hunter2" + strings.Repeat(" ", 4088) + "?\n"
	if c := replyOfSession(t, typedPrompt("Fix it"), agentLine(textPart(message))); c.Reply != "Done." {
		t.Errorf("reply = %q", c.Reply)
	}
}
