package conversations

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalOf_NamesWhatEachToolCallActsOn(t *testing.T) {
	cases := []struct {
		name string
		call toolCall
		want waitingOn
	}{
		{"a short command shows whole", toolCall{name: "Bash", input: toolInput{Command: "git status"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "git status"}},
		{"a long command shows its first words", toolCall{name: "Bash", input: toolInput{Command: "git push origin motechfr/permission-pill --force-with-lease"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "git push origin…"}},
		{"a command on many lines shows as one", toolCall{name: "Bash", input: toolInput{Command: "make lint &&\n  make test"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "make lint && make test"}},
		{"a leading cd step is left out", toolCall{name: "Bash", input: toolInput{Command: "cd /Users/you/Code/kombo-webhooks-v2/apps/engine && mise x -- pnpm typecheck"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "mise x -- pnpm typecheck"}},
		{"a cd step joined with a semicolon keeps every later step", toolCall{name: "Bash", input: toolInput{Command: "cd /tmp;./deploy-prod;echo done"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "./deploy-prod;echo done"}},
		{"a command that is only a cd keeps it", toolCall{name: "Bash", input: toolInput{Command: "cd /tmp"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "cd /tmp"}},
		{"a short first word before a long one is cut inside the long one", toolCall{name: "Bash", input: toolInput{Command: "cat /Users/you/Code/an-organisation/a-repo/README.md"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "cat /Users/you/Code/an-organi…"}},
		{"one long word is cut inside", toolCall{name: "Bash", input: toolInput{Command: "./scripts/an-extremely-long-script-name-that-goes-on.sh"}},
			waitingOn{For: waitPermission, Tool: "Bash", Detail: "./scripts/an-extremely-long-s…"}},
		{"an edit shows the file name", toolCall{name: "Edit", input: toolInput{FilePath: "/Users/you/Code/kombo/src/dashboard.tsx"}},
			waitingOn{For: waitPermission, Tool: "Edit", Detail: "dashboard.tsx"}},
		{"a write shows the file name", toolCall{name: "Write", input: toolInput{FilePath: "/tmp/notes.md"}},
			waitingOn{For: waitPermission, Tool: "Write", Detail: "notes.md"}},
		{"a read shows the file name", toolCall{name: "Read", input: toolInput{FilePath: "/Users/you/Code/app/.env.example"}},
			waitingOn{For: waitPermission, Tool: "Read", Detail: ".env.example"}},
		{"a notebook edit shows the notebook", toolCall{name: "NotebookEdit", input: toolInput{NotebookPath: "/tmp/analysis.ipynb"}},
			waitingOn{For: waitPermission, Tool: "NotebookEdit", Detail: "analysis.ipynb"}},
		{"a fetch shows the domain only", toolCall{name: "WebFetch", input: toolInput{URL: "https://github.com:443/Chemaclass/agnostic-ai/pulls?state=open"}},
			waitingOn{For: waitPermission, Tool: "WebFetch", Detail: "github.com"}},
		{"a connector tool shows its server and action", toolCall{name: "mcp__claude_ai_Notion__notion-fetch"},
			waitingOn{For: waitPermission, Tool: "mcp__claude_ai_Notion__notion-fetch", Detail: "Notion fetch"}},
		{"a connector registered by id is named from its action", toolCall{name: "mcp__bfe9f18c-b8fd-43ae-b28d-268779cd446a__notion-fetch"},
			waitingOn{For: waitPermission, Tool: "mcp__bfe9f18c-b8fd-43ae-b28d-268779cd446a__notion-fetch", Detail: "Notion fetch"}},
		{"a connector with two words keeps them", toolCall{name: "mcp__claude_ai_Google_Calendar__list_events"},
			waitingOn{For: waitPermission, Tool: "mcp__claude_ai_Google_Calendar__list_events", Detail: "Google Calendar list events"}},
		{"a plugin's server drops the plugin name", toolCall{name: "mcp__plugin_postman_postman__authenticate"},
			waitingOn{For: waitPermission, Tool: "mcp__plugin_postman_postman__authenticate", Detail: "Postman authenticate"}},
		{"a local server keeps its name", toolCall{name: "mcp__kombo-dev__query_database"},
			waitingOn{For: waitPermission, Tool: "mcp__kombo-dev__query_database", Detail: "Kombo-dev query database"}},
		{"a tool with no rule has no detail", toolCall{name: "Glob", input: toolInput{FilePath: "/tmp/x"}},
			waitingOn{For: waitPermission, Tool: "Glob"}},
		{"a call with no input has no detail", toolCall{name: "Bash"},
			waitingOn{For: waitPermission, Tool: "Bash"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := *approvalOf(tc.call); got != tc.want {
				t.Errorf("approvalOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestApprovalOf_WithholdsADetailThatMayHoldACredential(t *testing.T) {
	cases := map[string]toolCall{
		"a token in the words shown":   {name: "Bash", input: toolInput{Command: "export GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789 && git push"}},
		"a token past the words shown": {name: "Bash", input: toolInput{Command: "git push https://deploy:hunter2secretvalue@github.com/acme/app.git main"}},
		"a password word":              {name: "Bash", input: toolInput{Command: "mysql -u root --password=hunter2 app"}},
		"a sign-in in a fetched link":  {name: "WebFetch", input: toolInput{URL: "https://admin:hunter2secretvalue@example.com/"}},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			got := *approvalOf(call)
			if got != (waitingOn{For: waitPermission, Tool: call.name}) {
				t.Errorf("approvalOf = %+v, want the tool and no detail", got)
			}
		})
	}
}

func TestScan_SaysWhatALiveClaudeSessionWaitsFor(t *testing.T) {
	const (
		idPermission = "a1111111-1111-4111-8111-111111111111"
		idQuestion   = "a2222222-2222-4222-8222-222222222222"
		idDialog     = "a3333333-3333-4333-8333-333333333333"
		idAnswered   = "a4444444-4444-4444-8444-444444444444"
		idTwoCalls   = "a5555555-5555-4555-8555-555555555555"
		idSeenBefore = "a6666666-6666-4666-8666-666666666666"
		idCallsOnly  = "a7777777-7777-4777-8777-777777777777"
		idMCPShape   = "a8888888-8888-4888-8888-888888888888"
	)
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true}})
	project := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app")
	prompt := obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z", "message": obj{"role": "user", "content": "Ship it"}}
	said := func(text string, calls ...obj) obj {
		content := []obj{{"type": "text", "text": text}}
		content = append(content, calls...)
		return obj{"type": "assistant", "timestamp": "2026-11-07T08:01:00.000Z", "message": obj{"role": "assistant", "content": content}}
	}
	bash := func(id, command string) obj {
		return obj{"type": "tool_use", "id": id, "name": "Bash", "input": obj{"command": command, "description": "Push"}}
	}
	result := func(id string) obj {
		return obj{"type": "user", "timestamp": "2026-11-07T08:02:00.000Z", "message": obj{"role": "user", "content": []obj{{"type": "tool_result", "tool_use_id": id, "content": "ok"}}}}
	}
	write(t, filepath.Join(project, idPermission+".jsonl"), lines(prompt, said("Pushing the branch.", bash("call-1", "git push origin main"))))
	write(t, filepath.Join(project, idQuestion+".jsonl"), lines(prompt, said("Which one?", obj{"type": "tool_use", "id": "ask-1", "name": "AskUserQuestion", "input": obj{"questions": []obj{{"question": "Overwrite the files?"}}}})))
	write(t, filepath.Join(project, idDialog+".jsonl"), lines(prompt, said("Done.")))
	write(t, filepath.Join(project, idAnswered+".jsonl"), lines(prompt, said("Checking.", bash("call-1", "git status")), result("call-1"), said("Clean.")))
	write(t, filepath.Join(project, idTwoCalls+".jsonl"), lines(prompt, said("Two steps.", bash("call-1", "make test"), bash("call-2", "git push")), result("call-1")))
	write(t, filepath.Join(project, idSeenBefore+".jsonl"), lines(prompt, said("Old step.", bash("call-1", "rm -rf build")), prompt, said("New turn.")))
	callsOnly := obj{"type": "assistant", "timestamp": "2026-11-07T08:01:00.000Z", "message": obj{"role": "assistant", "content": []obj{bash("call-1", "rm -rf build")}}}
	write(t, filepath.Join(project, idCallsOnly+".jsonl"), lines(prompt, callsOnly, prompt))
	// An MCP tool may send a "command" that is no string: the message and its call still count.
	mcp := obj{"type": "tool_use", "id": "call-1", "name": "mcp__claude_ai_Notion__notion-fetch", "input": obj{"command": []string{"fetch", "page"}, "url": obj{"id": 1}}}
	write(t, filepath.Join(project, idMCPShape+".jsonl"), lines(prompt, said("Fetching the page.", mcp)))
	sessions := filepath.Join(stores.ClaudeDir, "sessions")
	write(t, filepath.Join(sessions, "7.json"), `{"pid":7,"sessionId":"`+idCallsOnly+`","status":"waiting","waitingFor":"permission prompt"}`)
	write(t, filepath.Join(sessions, "8.json"), `{"pid":8,"sessionId":"`+idMCPShape+`","status":"waiting","waitingFor":"permission prompt"}`)
	write(t, filepath.Join(sessions, "1.json"), `{"pid":1,"sessionId":"`+idPermission+`","status":"waiting","waitingFor":"permission prompt","statusUpdatedAt":1794045600500}`)
	write(t, filepath.Join(sessions, "2.json"), `{"pid":2,"sessionId":"`+idQuestion+`","status":"waiting","waitingFor":"input needed"}`)
	write(t, filepath.Join(sessions, "3.json"), `{"pid":3,"sessionId":"`+idDialog+`","status":"waiting","waitingFor":"dialog open"}`)
	// A start in a shape the scan does not read leaves only the start unknown.
	write(t, filepath.Join(sessions, "4.json"), `{"pid":4,"sessionId":"`+idAnswered+`","status":"waiting","waitingFor":"permission prompt","statusUpdatedAt":"soon"}`)
	write(t, filepath.Join(sessions, "5.json"), `{"pid":5,"sessionId":"`+idTwoCalls+`","status":"waiting","waitingFor":"sandbox request"}`)
	write(t, filepath.Join(sessions, "6.json"), `{"pid":6,"sessionId":"`+idSeenBefore+`","status":"waiting","waitingFor":"permission prompt"}`)

	view := scanner.Scan(context.Background(), ScanOptions{})

	want := map[string]string{
		idPermission: `{
  "for": "permission",
  "tool": "Bash",
  "detail": "git push origin main"
}`,
		idQuestion: `{
  "for": "answer"
}`,
		// A dialog says nothing about what it asks.
		idDialog: `null`,
		// Every call has its result: the call that waits is not known.
		idAnswered: `{
  "for": "permission"
}`,
		// The oldest call with no result waits first.
		idTwoCalls: `{
  "for": "permission",
  "tool": "Bash",
  "detail": "git push"
}`,
		// A call from before the latest prompt is not the one that waits.
		idSeenBefore: `{
  "for": "permission"
}`,
		// Also when the agent said nothing but the call before that prompt.
		idCallsOnly: `{
  "for": "permission"
}`,
		idMCPShape: `{
  "for": "permission",
  "tool": "mcp__claude_ai_Notion__notion-fetch",
  "detail": "Notion fetch"
}`,
	}
	if since, waits := byID(t, view, idPermission).PermissionWait(); !waits || !since.Equal(time.UnixMilli(1794045600500)) {
		t.Errorf("the permission wait began %v (waits %t), want the session's statusUpdatedAt", since, waits)
	}
	if since, waits := byID(t, view, idAnswered).PermissionWait(); !waits || !since.IsZero() {
		t.Errorf("a start that does not parse = %v (waits %t)", since, waits)
	}
	if _, waits := byID(t, view, idQuestion).PermissionWait(); waits {
		t.Error("a question reads as a permission wait")
	}
	if reply := byID(t, view, idMCPShape).Reply; reply != "Fetching the page." {
		t.Errorf("the reply beside an MCP call of any shape = %q", reply)
	}
	for id, wantJSON := range want {
		if got := asJSON(t, byID(t, view, id).Waiting); got != wantJSON {
			t.Errorf("%s waiting =\n%s\nwant\n%s", id, got, wantJSON)
		}
	}
}

func TestScan_SaysNothingOfWhatAClaudeSessionThatDoesNotWaitWaitsFor(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{1: true}})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z", "message": obj{"role": "user", "content": "Ship it"}},
		obj{"type": "assistant", "timestamp": "2026-11-07T08:01:00.000Z", "message": obj{"role": "assistant", "content": []obj{{"type": "tool_use", "id": "call-1", "name": "Bash", "input": obj{"command": "make test"}}}}},
	))
	// A running command has no result yet either, and asks for nothing.
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "1.json"), `{"pid":1,"sessionId":"`+idSafari+`","status":"busy"}`)

	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Waiting != nil {
		t.Errorf("a working session waits for %s", asJSON(t, c.Waiting))
	}
}

func TestCodexCloud_SaysWhatAThreadWaitsFor(t *testing.T) {
	const (
		approval = "01a1c27f-183f-73fb-b239-000000000011"
		input    = "01a1c27f-183f-73fb-b239-000000000012"
		both     = "01a1c27f-183f-73fb-b239-000000000013"
		busy     = "01a1c27f-183f-73fb-b239-000000000014"
		idle     = "01a1c27f-183f-73fb-b239-000000000015"
	)
	cloud := codexAPI(t, answerWith(http.StatusOK, `{"data": [
		{"id": "`+approval+`", "name": "a", "status": {"type": "active", "activeFlags": ["waitingOnApproval"]}, "recencyAt": 1794264400},
		{"id": "`+input+`", "name": "b", "status": {"type": "active", "activeFlags": ["waitingOnUserInput"]}, "recencyAt": 1794264400},
		{"id": "`+both+`", "name": "c", "status": {"type": "active", "activeFlags": ["waitingOnUserInput", "waitingOnApproval"]}, "recencyAt": 1794264400},
		{"id": "`+busy+`", "name": "d", "status": {"type": "active", "activeFlags": []}, "recencyAt": 1794264400},
		{"id": "`+idle+`", "name": "e", "status": {"type": "idle", "activeFlags": ["waitingOnApproval"]}, "recencyAt": 1794264400}
	]}`))

	rows, err := cloud.List(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	// The cloud names no tool call, so a permission comes without one.
	want := map[string]string{
		approval: `{
  "for": "permission"
}`,
		input: `{
  "for": "answer"
}`,
		both: `{
  "for": "permission"
}`,
		busy: `null`,
		idle: `null`,
	}
	for _, row := range rows {
		if got := asJSON(t, row.Waiting); got != want[row.ID] {
			t.Errorf("%s waiting = %s, want %s", row.ID, got, want[row.ID])
		}
	}
}
