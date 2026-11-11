package conversations

import (
	"context"
	"os"
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
	long := "## Summary\n\n- I changed **the cookie** in `middleware.go`.\n" + strings.Repeat("Then I ran the whole suite again. ", 60)

	shown := showTexts(texts{reply: long})

	if utf8.RuneCountInString(shown.reply) != replyRunes || !strings.HasSuffix(shown.reply, "…") {
		t.Errorf("reply has %d runes: %q", utf8.RuneCountInString(shown.reply), shown.reply)
	}
	if !strings.HasPrefix(shown.reply, "Summary I changed the cookie in middleware.go. Then") {
		t.Errorf("markup must not show, actual %q", shown.reply)
	}
	reply, _ := agentSaid(long, "")
	if len(reply) > replyKeptBytes || len(reply) < replyKeptBytes-40 || !strings.HasPrefix(long, reply) || long[len(reply)] != ' ' {
		t.Errorf("a reader keeps whole words up to %d bytes, actual %d bytes: %q", replyKeptBytes, len(reply), reply)
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
		{"thread_id":"` + answered + `","item":"{\"type\":\"agentMessage\",\"text\":\"Done, the tests pass.\",\"phase\":\"final_answer\"}"},
		{"thread_id":"` + asking + `","item":"{\"type\":\"agentMessage\",\"text\":\"Two queues fit.\",\"phase\":\"final_answer\",\"questions\":[{\"title\":\"Which queue should the jobs use?\",\"options\":[]}]}"},
		{"thread_id":"` + working + `","item":""}
	]`
	scan := func(answers map[string]string) View {
		scanner, stores := testScanner(t, fakeStores(t, answers), fakeProcesses{})
		write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
		write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")
		return scanner.Scan(context.Background(), ScanOptions{})
	}

	view := scan(map[string]string{"state_5.sqlite|FROM threads": threads, "thread_history_1.sqlite|userMessage": prompts, "thread_history_1.sqlite|thread_turns": replies})

	for id, want := range map[string]string{answered: "Done, the tests pass.", asking: "Which queue should the jobs use?", working: ""} {
		if c := byID(t, view, id); c.Reply != want {
			t.Errorf("thread %s: reply = %q, want %q", id[:4], c.Reply, want)
		}
	}
	failed := scan(map[string]string{"state_5.sqlite|FROM threads": threads, "thread_history_1.sqlite|userMessage": prompts})
	if c := byID(t, failed, working); c.Reply != "" || c.Prompt != "Now the API page" || len(failed.Conversations) != 3 {
		t.Errorf("a failed reply query must leave the rows and prompts: %s", asJSON(t, failed.Conversations))
	}
}

func TestScan_ShowsNoReplyForCursorAndCloudRows(t *testing.T) {
	agent := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{}), fakeProcesses{})
	write(t, filepath.Join(stores.CursorDir, "projects", "p1", "agent-transcripts", agent, agent+".jsonl"), lines(
		obj{"role": "user", "message": obj{"content": []obj{textPart("<user_query>Fix the CI job</user_query>")}}},
		obj{"role": "assistant", "message": obj{"content": []obj{textPart("The CI job passes now.")}}},
	))
	write(t, filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db"), "")
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
