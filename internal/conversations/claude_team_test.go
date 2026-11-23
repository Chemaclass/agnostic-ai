package conversations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The ids the fixtures in testdata/claude-teams name.
const (
	inProcessLead     = "ad3d3020-5195-4a73-a9d3-5bc3e45b66b9"
	inProcessTeam     = "session-ad3d3020"
	inProcessTeammate = "alegolas-3905-f554659539adf72a"
	inProcessSpawn    = "toolu_01HZAyL2VrhwG1myrZeG98rs"
	tmuxLead          = "820909bc-14f2-48f6-b4da-df2d65f577ee"
	tmuxTeam          = "session-820909bc"
	tmuxTeammate      = "echo@session-820909bc"
	tmuxSpawn         = "toolu_01ULGP38LAuNtWFLNsV6mzVe"
	swarmLead         = "0d2fda98-c29b-412d-9ef3-f77411c7c975"
)

func teamFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "claude-teams", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// teamLead writes a lead session whose transcript spawned the teammate in
// spawnFixture, and the team config in configFixture, edited by change.
// It returns the lead's transcript and a live session that started a
// minute before the spawn.
func teamLead(t *testing.T, stores Stores, lead, team, spawnCall, spawnFixture, configFixture string, change func(obj)) (string, liveSession) {
	t.Helper()
	file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", lead+".jsonl")
	spawn := teamFixture(t, spawnFixture)
	var line struct {
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(spawn), &line); err != nil {
		t.Fatal(err)
	}
	dispatch := workerDispatch(spawnCall)
	dispatch["timestamp"] = line.Timestamp
	write(t, file, lines(dispatch)+spawn)
	writeTeam(t, stores, team, configFixture, change)
	spawned, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
	return file, liveSession{startedAt: spawned.Add(-time.Minute).UnixMilli()}
}

func writeTeam(t *testing.T, stores Stores, team, configFixture string, change func(obj)) {
	t.Helper()
	var config obj
	if err := json.Unmarshal([]byte(teamFixture(t, configFixture)), &config); err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(config)
	}
	write(t, filepath.Join(stores.ClaudeDir, "teams", team, "config.json"), asJSON(t, config))
}

// teammateTranscript writes an in-process teammate's meta and transcript
// beside its lead's, the transcript written after the spawn.
func teammateTranscript(t *testing.T, lead string, transcript ...any) string {
	t.Helper()
	dir := claudeSubagentsDir(lead)
	write(t, filepath.Join(dir, "agent-"+inProcessTeammate+".meta.json"), teamFixture(t, "in-process-teammate.meta.json"))
	path := filepath.Join(dir, "agent-"+inProcessTeammate+".jsonl")
	write(t, path, lines(transcript...))
	return path
}

// at stamps a teammate's line some seconds after its spawn.
func at(line obj, seconds int) obj {
	spawned := time.Date(2026, 11, 22, 7, 20, 16, 71e6, time.UTC)
	line["timestamp"] = spawned.Add(time.Duration(seconds) * time.Second).Format(time.RFC3339Nano)
	line["isSidechain"] = true
	return line
}

func teammateMessage(seconds int) obj {
	return at(typedPrompt(`<teammate-message teammate_id="team-lead">Build it</teammate-message>`), seconds)
}

func teammateTurnEnd(seconds int) obj {
	line := agentLine(textPart("Done."))
	line["message"].(obj)["stop_reason"] = "end_turn"
	return at(line, seconds)
}

func TestClaudeTeam_AnInProcessTeammateWorksUntilItsTurnEndsAndAgainOnANewMessage(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	lead, session := teamLead(t, stores, inProcessLead, inProcessTeam, inProcessSpawn, "in-process-lead-spawn.jsonl", "in-process-team-config.json", nil)
	transcript := teammateTranscript(t, lead, teammateMessage(1))
	check := func(want int) []helper {
		t.Helper()
		work, _ := scanner.claudeWorkOf(lead, session)
		if work == nil || work.count() != want {
			t.Fatalf("working = %v, want %d", work, want)
		}
		return scanner.claudeHelpers(lead, work, map[string]bool{})
	}

	helpers := check(1)
	if len(helpers) != 1 {
		t.Fatalf("the working teammate is listed, actual %s", asJSON(t, helpers))
	}
	if got := helpers[0]; got.ID != inProcessTeammate || got.Title != "Build the subagent tree" || got.Kind != "legolas-3905" || got.Reach != reachRows || got.shownName() != "legolas-3905" {
		t.Errorf("the row carries the teammate's name and is reached by the rows below the prompt, actual %s name %q", asJSON(t, got), got.shownName())
	}
	appendWorkerEvent(t, transcript, teammateTurnEnd(2))
	if helpers := check(0); helpers != nil {
		t.Errorf("an idle teammate leaves the list, actual %s", asJSON(t, helpers))
	}
	appendWorkerEvent(t, transcript, teammateMessage(3))
	check(1)
}

func TestClaudeTeam_ATeammateItsTeamNoLongerListsHasStopped(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	drop := func(config obj) {
		config["members"] = config["members"].([]any)[:1]
	}
	lead, session := teamLead(t, stores, inProcessLead, inProcessTeam, inProcessSpawn, "in-process-lead-spawn.jsonl", "in-process-team-config.json", drop)
	// A stop leaves its transcript mid-turn.
	teammateTranscript(t, lead, teammateMessage(1))

	if work, _ := scanner.claudeWorkOf(lead, session); work == nil || work.count() != 0 {
		t.Errorf("a teammate its team left out is not counted, actual %v", work)
	}
}

func TestClaudeTeam_ATeamOfAnotherLeadOrNoTeamCountsNoTeammate(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	otherLead := func(config obj) {
		config["leadSessionId"] = swarmLead
		config["members"] = config["members"].([]any)[:1]
	}
	lead, session := teamLead(t, stores, inProcessLead, inProcessTeam, inProcessSpawn, "in-process-lead-spawn.jsonl", "in-process-team-config.json", otherLead)
	teammateTranscript(t, lead, teammateMessage(1))

	if work, _ := scanner.claudeWorkOf(lead, session); work == nil || work.count() != 0 {
		t.Errorf("a team config of another lead is ignored, and a teammate no team vouches for is not counted, actual %v", work)
	}
	if err := os.Remove(filepath.Join(stores.ClaudeDir, "teams", inProcessTeam, "config.json")); err != nil {
		t.Fatal(err)
	}
	if work, _ := scanner.claudeWorkOf(lead, session); work == nil || work.count() != 0 {
		t.Errorf("a teammate whose team is missing is not counted, actual %v", work)
	}
	write(t, filepath.Join(stores.ClaudeDir, "teams", inProcessTeam, "config.json"), `{"leadSessionId":`)
	if work, watched := scanner.claudeWorkOf(lead, session); work != nil || !slices.Contains(watched, filepath.Join(stores.ClaudeDir, "teams", inProcessTeam, "config.json")) {
		t.Errorf("a team config caught halfway keeps the count unknown and is watched, actual %v", work)
	}
}

func TestClaudeTeam_ASubagentATeammateStartedIsNestedUnderIt(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	lead, session := teamLead(t, stores, inProcessLead, inProcessTeam, inProcessSpawn, "in-process-lead-spawn.jsonl", "in-process-team-config.json", nil)
	var nested struct {
		ToolUseID string `json:"toolUseId"`
	}
	if err := json.Unmarshal([]byte(teamFixture(t, "nested-subagent.meta.json")), &nested); err != nil {
		t.Fatal(err)
	}
	teammate := teammateTranscript(t, lead, teammateMessage(1), at(sidechainDispatch("Agent", nested.ToolUseID), 2))
	dir := claudeSubagentsDir(lead)
	write(t, filepath.Join(dir, "agent-anested.meta.json"), teamFixture(t, "nested-subagent.meta.json"))
	write(t, filepath.Join(dir, "agent-anested.jsonl"), "")

	work, _ := scanner.claudeWorkOf(lead, session)
	if work == nil || work.count() != 2 {
		t.Fatalf("the teammate and its subagent work, actual %v", work)
	}
	helpers := scanner.claudeHelpers(lead, work, map[string]bool{})
	index := slices.IndexFunc(helpers, func(h helper) bool { return h.ID == "anested" })
	if index < 0 {
		t.Fatalf("the nested subagent is listed, actual %s", asJSON(t, helpers))
	}
	if got := helpers[index]; got.ParentID != inProcessTeammate || got.Reach != reachNested || got.ParentName != "legolas-3905" {
		t.Errorf("the nested subagent names the teammate it runs inside, actual %s", asJSON(t, got))
	}

	appendWorkerEvent(t, lead, at(workerDispatch("other-call"), 3))
	appendWorkerEvent(t, lead, at(workerResult("other-call", obj{"status": "async_launched", "agentId": "other"}), 4))
	appendWorkerEvent(t, teammate, at(sidechainDispatch("TaskStop", "stop-other"), 5))
	appendWorkerEvent(t, teammate, at(sidechainResult("stop-other", `{"task_id":"other"}`), 6))
	writeTeam(t, stores, inProcessTeam, "in-process-team-config.json", func(config obj) {
		config["members"] = config["members"].([]any)[:1]
	})
	work, _ = scanner.claudeWorkOf(lead, session)
	if work == nil || work.count() != 0 {
		t.Fatalf("removing a teammate also clears its unfinished subagent, actual %v", work)
	}
	if helpers := scanner.claudeHelpers(lead, work, map[string]bool{}); len(helpers) != 0 {
		t.Errorf("the stopped teammate and its subagent leave the list, actual %s", asJSON(t, helpers))
	}
	if working, seen := work.agents["other"]; !seen || working {
		t.Errorf("the teammate's completed stop of a separate agent stays recorded, actual %v", work.agents)
	}
}

func TestClaudeTeam_APaneTeammateWorksWhileItsTeamMarksItActive(t *testing.T) {
	for name, tc := range map[string]struct {
		change    func(obj)
		wantCount int
		wantReach helperReach
	}{
		"idle, as captured": {nil, 0, ""},
		"active": {func(config obj) {
			config["members"].([]any)[1].(map[string]any)["isActive"] = true
		}, 1, reachPane},
		"active with a malformed pane id": {func(config obj) {
			member := config["members"].([]any)[1].(map[string]any)
			member["isActive"], member["tmuxPaneId"] = true, "%1; kill-server"
		}, 1, reachRows},
		"active in a team of another lead": {func(config obj) {
			config["members"].([]any)[1].(map[string]any)["isActive"] = true
			config["leadSessionId"] = swarmLead
		}, 0, ""},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, nil, fakeProcesses{})
			lead, session := teamLead(t, stores, tmuxLead, tmuxTeam, tmuxSpawn, "tmux-lead-spawn.jsonl", "tmux-team-config.json", tc.change)

			work, _ := scanner.claudeWorkOf(lead, session)
			if work == nil || work.count() != tc.wantCount {
				t.Fatalf("working = %v, want %d", work, tc.wantCount)
			}
			helpers := scanner.claudeHelpers(lead, work, map[string]bool{})
			if tc.wantCount == 0 {
				if helpers != nil {
					t.Errorf("no helper is listed, actual %s", asJSON(t, helpers))
				}
				return
			}
			if len(helpers) != 1 || helpers[0].ID != tmuxTeammate || helpers[0].Title != "echo" || helpers[0].Reach != tc.wantReach {
				t.Errorf("the pane teammate is listed by its name, actual %s", asJSON(t, helpers))
			}
			if got := helpersJSON(t, helpers); strings.Contains(got, "%1") || strings.Contains(got, "member") {
				t.Errorf("pane ids stay out of the JSON, actual %s", got)
			}
		})
	}
}

// swarmTable is a pane teammate's process under Claude Code's own tmux
// server, as the fixtures capture it, with a terminal tab running a
// tmux client of that server when attached.
func swarmTable(t *testing.T, attached bool) fakeProcesses {
	t.Helper()
	table := []ProcessInfo{
		{PID: 19211, PPID: 1, Stat: "Ss", Command: "tmux"},
		{PID: 19212, PPID: 19211, Stat: "Ss+", TTY: "/dev/ttys006", Command: "/Users/me/.local/share/claude/versions/2.1.330"},
		{PID: 22657, PPID: 1, Stat: "S", Started: "Mon Nov 16 09:00:00 2026", Command: terminalAppPath},
	}
	if attached {
		table = append(table, terminalAttach("/dev/ttys009", 31000, "Mon Nov 16 09:00:00 2026")...)
		table[len(table)-1].Command = "tmux"
	}
	return fakeProcesses{table: table, args: map[int]string{
		19211: "tmux -L claude-swarm-18267 new-session -d -s claude-swarm -n swarm-view",
		19212: strings.TrimSpace(teamFixture(t, "swarm-teammate-ps.txt")),
	}}
}

func TestOpenTeammatePane_SelectsThePaneAndItsTerminalTab(t *testing.T) {
	processes := swarmTable(t, true)
	launcher := &fakeLauncher{focusOK: true, tmuxClient: 31000}
	lead := Conversation{Tool: ToolClaude, ID: swarmLead, Folder: "/tmp/app"}

	opening := openTeammatePane(context.Background(), lead, "echo@session-0d2fda98", processes, launcher)

	if !opening.shown || opening.attach != "" {
		t.Errorf("the pane and its terminal tab come to the front, actual %+v", opening)
	}
	want := []string{"tmux 19211 /dev/ttys006", "focus Terminal /dev/ttys009"}
	if !slices.Equal(launcher.calls, want) {
		t.Errorf("calls = %v, want %v", launcher.calls, want)
	}
}

func TestOpenTeammatePane_FindsATeammateAShellInThePaneStarted(t *testing.T) {
	processes := swarmTable(t, true)
	processes.table[1].PPID = 19300
	processes.table = append(processes.table, ProcessInfo{PID: 19300, PPID: 19211, Stat: "Ss", TTY: "/dev/ttys006", Command: "-zsh"})
	launcher := &fakeLauncher{focusOK: true, tmuxClient: 31000}

	opening := openTeammatePane(context.Background(), Conversation{Tool: ToolClaude, ID: swarmLead}, "echo@session-0d2fda98", processes, launcher)

	if !opening.shown || !slices.Contains(launcher.calls, "tmux 19211 /dev/ttys006") {
		t.Errorf("the pane of a teammate under a shell comes to the front, actual %+v calls %v", opening, launcher.calls)
	}
}

func TestReopenHelper_AJoinedOpenGetsOnlyItsOwnPaneAndAttachCommand(t *testing.T) {
	chat := Conversation{App: AppCLI, Open: Route{Kind: RouteFocus}, Helpers: helperTree()}
	out := openOutcome{found: true, chat: chat, reply: OpenReply{Opened: true, Message: "Switched."}, attach: "tmux -L s attach", attachFor: "pane"}

	if reply, _ := out.replyFor("under-pane"); reply.Command != "tmux -L s attach" || !strings.Contains(reply.Message, "The pane of 'echo'") {
		t.Errorf("a subagent inside the pane gets its owner's attach command, actual %q %q", reply.Message, reply.Command)
	}
	if reply, _ := out.replyFor("one"); reply.Command != "" || strings.Contains(reply.Message, "tmux") {
		t.Errorf("a subagent outside the pane gets no attach command, actual %q %q", reply.Message, reply.Command)
	}
	out = openOutcome{found: true, chat: chat, reply: OpenReply{Opened: true, Message: "Switched."}, pane: "pane"}
	if reply, _ := out.replyFor("one"); reply.Reached != reachedParent || reply.Message != "Switched. Its subagent is 'Level one'." {
		t.Errorf("with another helper's pane in front, a joined open only names its subagent, actual %+v", reply)
	}
	if reply, _ := out.replyFor("gone"); reply.Message != "Switched. That subagent has finished." {
		t.Errorf("with another helper's pane in front, a finished subagent says only that, actual %+v", reply)
	}
}

func TestOpenTeammatePane_APaneNoTerminalShowsGivesTheAttachCommand(t *testing.T) {
	launcher := &fakeLauncher{}
	lead := Conversation{Tool: ToolClaude, ID: swarmLead}

	opening := openTeammatePane(context.Background(), lead, "echo@session-0d2fda98", swarmTable(t, false), launcher)

	if opening.shown || opening.attach != "tmux -L claude-swarm-18267 attach" {
		t.Errorf("an unattached server gives its attach command, actual %+v", opening)
	}
}

func TestOpenTeammatePane_FindsOnlyTheMemberOfThisLead(t *testing.T) {
	for name, member := range map[string]string{
		"another member":           "other@session-0d2fda98",
		"the member of other lead": "echo@session-0d2fda98",
	} {
		t.Run(name, func(t *testing.T) {
			launcher := &fakeLauncher{tmuxClient: 31000}
			lead := Conversation{Tool: ToolClaude, ID: swarmLead}
			if name == "the member of other lead" {
				lead.ID = tmuxLead
			}

			opening := openTeammatePane(context.Background(), lead, member, swarmTable(t, true), launcher)

			if opening.shown || len(launcher.calls) != 0 {
				t.Errorf("no pane is touched, actual %+v calls %v", opening, launcher.calls)
			}
		})
	}
}

func TestOpenTeammatePane_OnlyOneClaudeProcessMatches(t *testing.T) {
	args := func(t *testing.T) string {
		t.Helper()
		return strings.TrimSpace(teamFixture(t, "swarm-teammate-ps.txt"))
	}
	for name, change := range map[string]func(t *testing.T, p *fakeProcesses){
		"a second Claude process with the same flags": func(t *testing.T, p *fakeProcesses) {
			t.Helper()
			p.table = append(p.table, ProcessInfo{PID: 19400, PPID: 19211, Stat: "Ss+", TTY: "/dev/ttys007", Command: p.table[1].Command})
			p.args[19400] = args(t)
		},
		"a program other than Claude": func(t *testing.T, p *fakeProcesses) {
			t.Helper()
			p.table[1].Command = "sh"
			p.args[19212] = "sh -c echo " + args(t)
		},
		"a flag given twice": func(t *testing.T, p *fakeProcesses) {
			t.Helper()
			p.args[19212] = args(t) + " --agent-id other@session-0d2fda98"
		},
	} {
		t.Run(name, func(t *testing.T) {
			processes := swarmTable(t, true)
			change(t, &processes)
			launcher := &fakeLauncher{focusOK: true, tmuxClient: 31000}

			opening := openTeammatePane(context.Background(), Conversation{Tool: ToolClaude, ID: swarmLead}, "echo@session-0d2fda98", processes, launcher)

			if opening.shown || len(launcher.calls) != 0 {
				t.Errorf("no pane is touched, so the lead opens, actual %+v calls %v", opening, launcher.calls)
			}
		})
	}
}

func TestClaudeProgram(t *testing.T) {
	for command, want := range map[string]bool{
		"/Users/me/.local/share/claude/versions/2.1.330 --agent-id a":       true,
		"/opt/homebrew/bin/claude --agent-id a":                             true,
		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js":       true,
		"node /tmp/other.js --agent-id a":                                   false,
		"/Users/John Doe/.local/share/claude/versions/2.1.330 --agent-id a": true,
		"sh -c claude --agent-id a":                                         false,
		"/usr/bin/claude-wrapper --agent-id a":                              false,
		"":                                                                  false,
	} {
		if got := claudeProgram(strings.Fields(command)); got != want {
			t.Errorf("claudeProgram(%q) = %v, want %v", command, got, want)
		}
	}
}

func TestTmuxAttachCommand_NamesOnlyASafeSocket(t *testing.T) {
	for command, want := range map[string]string{
		"tmux -L claude-swarm-18267 new-session -d": "tmux -L claude-swarm-18267 attach",
		"tmux new-session -d":                       "",
		"tmux -L a;b new-session":                   "",
		"tmux -S /tmp/x new-session":                "",
	} {
		if got := tmuxAttachCommand(command); got != want {
			t.Errorf("tmuxAttachCommand(%q) = %q, want %q", command, got, want)
		}
	}
}

// paneLeadScanner is a live lead in a Terminal tab whose pane teammate
// works in Claude Code's own tmux server, with a client in another tab
// when attached.
func paneLeadScanner(t *testing.T, attached bool) *Scanner {
	t.Helper()
	processes := swarmTable(t, attached)
	processes.args[19212] = strings.TrimSpace(teamFixture(t, "tmux-teammate-ps.txt"))
	processes.table = append(processes.table, terminalAttach("/dev/ttys004", 77, "Mon Nov 16 16:43:20 2026")...)
	processes.alive = map[int]bool{77: true}
	scanner, stores := testScanner(t, nil, processes)
	active := func(config obj) { config["members"].([]any)[1].(map[string]any)["isActive"] = true }
	lead, session := teamLead(t, stores, tmuxLead, tmuxTeam, tmuxSpawn, "tmux-lead-spawn.jsonl", "tmux-team-config.json", active)
	appendWorkerEvent(t, lead, obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-22T07:23:30.000Z", "message": obj{"role": "user", "content": "Start a team"}})
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77.json"),
		`{"pid":77,"sessionId":"`+tmuxLead+`","procStart":"","status":"busy","startedAt":`+asJSON(t, session.startedAt)+`}`)
	return scanner
}

func TestReopenHelper_SwitchesToAPaneTeammatesOwnPane(t *testing.T) {
	scanner := paneLeadScanner(t, true)
	launcher := &fakeLauncher{focusOK: true, tmuxClient: 31000}

	reply, found := scanner.ReopenHelper(context.Background(), ToolClaude, tmuxLead, tmuxTeammate, ScanOptions{}, launcher)

	if !found || !reply.Opened || reply.Reached != reachedSubagent || reply.Helper == nil || reply.Helper.ID != tmuxTeammate {
		t.Fatalf("the teammate's pane comes to the front, actual %+v calls %v", reply, launcher.calls)
	}
	if want := []string{"tmux 19211 /dev/ttys006", "focus Terminal /dev/ttys009"}; !slices.Equal(launcher.calls, want) {
		t.Errorf("calls = %v, want %v", launcher.calls, want)
	}
	if reply.Message != "Switched to the Terminal tab that runs it." {
		t.Errorf("no note follows its own pane, actual %q", reply.Message)
	}
}

func TestReopenHelper_FocusesTheLeadWhenNoTerminalShowsThePane(t *testing.T) {
	scanner := paneLeadScanner(t, false)
	launcher := &fakeLauncher{focusOK: true}

	reply, found := scanner.ReopenHelper(context.Background(), ToolClaude, tmuxLead, tmuxTeammate, ScanOptions{}, launcher)

	if !found || !reply.Opened || reply.Reached != reachedParent {
		t.Fatalf("the lead's tab comes to the front, actual %+v calls %v", reply, launcher.calls)
	}
	if want := []string{"tmux 19211 /dev/ttys006", "focus Terminal /dev/ttys004"}; !slices.Equal(launcher.calls, want) {
		t.Errorf("calls = %v, want %v", launcher.calls, want)
	}
	if reply.Command != "tmux -L claude-swarm-18267 attach" || !strings.Contains(reply.Message, "no terminal shows") {
		t.Errorf("the note gives the attach command, actual %q %q", reply.Message, reply.Command)
	}
}
