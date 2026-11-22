package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Claude Code agent teams. A lead session spawns teammates with its
// Agent tool: one in its own process, which writes beside the lead's
// subagents, or one in a tmux pane, which is a process of its own.
// ~/.claude/teams/<team>/config.json lists the members that run. None of
// this is a public contract, so each field is checked, and a shape that
// cannot be read keeps what the lead's transcript says.

// inProcessPane is the pane a teammate inside the lead's process names.
const inProcessPane = "in-process"

// claudeTeammate is what a lead's transcript says of a teammate it spawned.
type claudeTeammate struct {
	at time.Time
	// name is the teammate's name; team is its team's name, and member
	// its id there, such as reviewer@session-0d2fda98.
	name, team, member string
	// kind is the agent type it runs as.
	kind string
	// pane is true for a teammate the spawn placed in a terminal pane.
	pane bool
}

// claudeTeammateState is a teammate with whether it works now. paneID is
// the tmux pane its team lists for a pane teammate, when well formed.
type claudeTeammateState struct {
	claudeTeammate
	working bool
	paneID  string
}

// claudeTeamConfig is the part of a team's config.json read here.
type claudeTeamConfig struct {
	LeadSessionID string             `json:"leadSessionId"`
	Members       []claudeTeamMember `json:"members"`
}

type claudeTeamMember struct {
	AgentID     string `json:"agentId"`
	BackendType string `json:"backendType"`
	TmuxPaneID  string `json:"tmuxPaneId"`
	// IsActive is false while a pane teammate waits for work.
	IsActive *bool `json:"isActive"`
}

// plainName reports whether name is 1 to limit letters, digits, dots,
// hyphens or underscores, not starting with a dot: it stays inside a
// folder and is safe to show in a command.
func plainName(name string, limit int) bool {
	if name == "" || len(name) > limit || name[0] == '.' {
		return false
	}
	return strings.IndexFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '-' && r != '_'
	}) < 0
}

// tmuxPaneID reports whether id is a tmux pane id, such as %12.
func tmuxPaneID(id string) bool {
	digits := strings.TrimPrefix(id, "%")
	return len(digits) > 0 && len(digits) < len(id) && strings.Trim(digits, "0123456789") == ""
}

// claudeTeamPath is a team's config file, or "" for a name that could
// reach outside the teams folder.
func (s *Scanner) claudeTeamPath(team string) string {
	if !plainName(team, 128) {
		return ""
	}
	return filepath.Join(s.Stores.ClaudeDir, "teams", team, "config.json")
}

// claudeTeamOf reads the members of lead's team. None are listed when
// the config is missing or names another lead. known is false when it
// exists but cannot be read or parsed, as in a write caught halfway:
// that says nothing either way.
func (s *Scanner) claudeTeamOf(team, lead string) (members map[string]claudeTeamMember, known bool) {
	path := s.claudeTeamPath(team)
	if path == "" {
		return nil, true
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	var config claudeTeamConfig
	if err != nil || json.Unmarshal(raw, &config) != nil {
		return nil, false
	}
	if config.LeadSessionID != lead {
		return nil, true
	}
	members = make(map[string]claudeTeamMember, len(config.Members))
	for _, member := range config.Members {
		members[member.AgentID] = member
	}
	return members, true
}

// claudeTeammateStateOf says whether a teammate of lead works. Its team
// lists the members that run: one it does not list was stopped or shut
// down, and a stop leaves no other trace a scan can read. A teammate
// whose team is missing, or names another lead, is not counted, as
// before teams were read. known is false when the team cannot be read.
// A pane teammate works while its team marks it active. One in the
// lead's process works by the newest of what the lead says of it,
// spawned or resumed, and its own transcript's turn.
func (s *Scanner) claudeTeammateStateOf(lead string, teammate claudeTeammate, said, turn nativeTaskEvent) (state claudeTeammateState, known bool) {
	state = claudeTeammateState{claudeTeammate: teammate}
	members, known := s.claudeTeamOf(teammate.team, lead)
	if !known {
		return state, false
	}
	member, listed := members[teammate.member]
	switch {
	case !listed:
	case teammate.pane:
		state.working = member.IsActive == nil || *member.IsActive
		if member.BackendType == "tmux" && tmuxPaneID(member.TmuxPaneID) {
			state.paneID = member.TmuxPaneID
		}
	default:
		event := said
		if !turn.at.IsZero() && !turn.at.Before(said.at) {
			event = turn
		}
		state.working = event.working
	}
	return state, true
}

// claudePaneHelpers lists the pane teammates of a session that work.
// They have no transcript beside the lead's, so their spawn names them.
func (s *Scanner) claudePaneHelpers(work *claudeWork) []helper {
	var helpers []helper
	for id, teammate := range work.teammates {
		if !teammate.pane || !teammate.working || !ValidAgentID(id) {
			continue
		}
		h := helper{
			Tool: ToolClaude, ID: id, Kind: teammate.kind, State: helperWorking,
			Title: s.helperTitle(teammate.name, ""), UpdatedAt: stamp(teammate.at), Reach: reachRows, name: teammate.name,
		}
		if teammate.paneID != "" {
			h.Reach = reachPane
			h.member = teammate.member
		}
		helpers = append(helpers, h)
	}
	return helpers
}

// teammatePane finds the process of a pane teammate of lead: a Claude
// Code process in a tmux pane whose arguments name both the member and
// its lead, each once. Claude Code 2.1.330 runs it as the pane's own
// process; one a shell in the pane started is found too. Any other
// program, or more than one match, finds none, so the lead opens
// instead. The pane id its team lists is never handed to tmux; the
// process says which pane it runs in.
func teammatePane(table []ProcessInfo, args func([]int) map[int]string, member, lead string) (int, bool) {
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	inTmux := func(process ProcessInfo) bool {
		for range maxAncestors {
			parent, found := byPID[process.PPID]
			if !found || process.PPID <= 1 {
				return false
			}
			if programOf(parent.Command) == "tmux" && parent.TTY == "" {
				return true
			}
			process = parent
		}
		return false
	}
	var panes []int
	for _, process := range table {
		if process.TTY != "" && inTmux(process) {
			panes = append(panes, process.PID)
		}
	}
	if len(panes) == 0 {
		return 0, false
	}
	found := 0
	for pid, command := range args(panes) {
		words := strings.Fields(command)
		if !claudeProgram(words) || onlyArgValue(words, "--agent-id") != member || onlyArgValue(words, "--parent-session-id") != lead {
			continue
		}
		if found != 0 {
			return 0, false
		}
		found = pid
	}
	return found, found != 0
}

// claudeProgram reports whether a command line runs Claude Code: its
// executable, named claude or kept in the native installer's versions
// folder, or node running the npm package. ps joins arguments with
// spaces, so the executable is every word before the first flag.
func claudeProgram(words []string) bool {
	if len(words) > 1 && filepath.Base(words[0]) == "node" {
		return strings.Contains(filepath.ToSlash(words[1]), "/@anthropic-ai/claude-code/")
	}
	end := slices.IndexFunc(words, func(word string) bool { return strings.HasPrefix(word, "-") })
	if end < 0 {
		end = len(words)
	}
	executable := filepath.ToSlash(strings.Join(words[:end], " "))
	return filepath.Base(executable) == "claude" || strings.Contains(executable, "/claude/versions/")
}

// onlyArgValue is the word after flag when flag appears once, or "".
func onlyArgValue(words []string, flag string) string {
	value, seen := "", 0
	for i, word := range words {
		if word != flag {
			continue
		}
		seen++
		if i+1 < len(words) {
			value = words[i+1]
		}
	}
	if seen != 1 {
		return ""
	}
	return value
}

// argValue is the word after flag in a command line, or "".
func argValue(command, flag string) string {
	words := strings.Fields(command)
	for i := 0; i+1 < len(words); i++ {
		if words[i] == flag {
			return words[i+1]
		}
	}
	return ""
}

// tmuxAttachCommand is the command that shows a tmux server no client
// shows, from the -L name its server started with. Empty when the server
// names no socket that way.
func tmuxAttachCommand(command string) string {
	name := argValue(command, "-L")
	if !plainName(name, 64) {
		return ""
	}
	return "tmux -L " + name + " attach"
}

// paneOpening is how an open of a pane teammate went.
type paneOpening struct {
	reply OpenReply
	// shown is true when the teammate's pane is in front.
	shown bool
	// attach, when set, is the command that shows a pane no terminal shows.
	attach string
}

// openTeammatePane selects a pane teammate's tmux pane and brings the
// terminal that shows it to the front. A pane in a tmux session no
// terminal shows is not shown; the caller opens the lead instead.
func openTeammatePane(ctx context.Context, c Conversation, member string, processes Processes, launcher Launcher) paneOpening {
	if shellSystem == "windows" {
		return paneOpening{}
	}
	table, err := processes.Table()
	if err != nil {
		return paneOpening{}
	}
	pid, found := teammatePane(table, processes.Args, member, c.ID)
	if !found {
		return paneOpening{}
	}
	host, _ := hostOf(table, pid)
	if host.App != "tmux" {
		return paneOpening{}
	}
	client, err := launcher.ShowTmuxPane(ctx, host.tmuxServer, host.TTY)
	if err != nil {
		return paneOpening{}
	}
	if client == 0 {
		return paneOpening{attach: tmuxAttachCommand(processes.Args([]int{host.tmuxServer})[host.tmuxServer])}
	}
	outer, _ := hostOf(table, client)
	if outer.TTY == "" || outer.App == "tmux" {
		return paneOpening{}
	}
	reply := switchToHost(ctx, c.Folder, placement{process: liveProcess{PID: pid}, host: outer}, processes, launcher, "")
	return paneOpening{reply: reply, shown: reply.Opened}
}
