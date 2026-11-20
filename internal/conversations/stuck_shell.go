package conversations

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

const defaultStuckShellAfter = 30 * time.Minute

func (opts ScanOptions) shellAfter() time.Duration {
	if opts.StuckShellAfter == nil {
		return defaultStuckShellAfter
	}
	return *opts.StuckShellAfter
}

func canStopShells() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

// Claude Code wraps Bash calls in a fresh shell that sources its snapshot
// and writes the resulting working directory to a temporary file.
var claudeShellWrapper = regexp.MustCompile(`(?s)^([^ ]+) -c source (.+?) 2>/dev/null \|\| true (.*?)&& eval ('.*') < /dev/null && pwd -P >\| (.+)$`)

func shellCommand(args string) string {
	parts := claudeShellWrapper.FindStringSubmatch(args)
	if len(parts) == 0 {
		return ""
	}
	switch filepath.Base(parts[1]) {
	case "bash", "zsh", "sh":
	default:
		return ""
	}
	snapshot, cwd := strings.Trim(parts[2], `"'`), strings.Trim(parts[5], `"'`)
	if filepath.Base(filepath.Dir(snapshot)) != "shell-snapshots" ||
		!strings.HasPrefix(filepath.Base(snapshot), "snapshot-") || !strings.HasSuffix(snapshot, ".sh") ||
		!strings.HasPrefix(filepath.Base(cwd), "claude-") || !strings.HasSuffix(cwd, "-cwd") {
		return ""
	}
	command := strings.NewReplacer(`'\''`, "'", `'"'"'`, "'").Replace(parts[4][1 : len(parts[4])-1])
	if strings.TrimSpace(command) == "" {
		return ""
	}
	return command
}

type claudeShellCall struct {
	command  string
	started  time.Time
	answered time.Time
	async    bool
}

func (events *claudeWorkerEvents) recordShellCall(id string, input json.RawMessage, when time.Time) {
	var bash struct {
		Command    string `json:"command"`
		Background bool   `json:"run_in_background"`
	}
	if id == "" || json.Unmarshal(input, &bash) != nil || bash.Command == "" {
		return
	}
	if events.shellCalls == nil {
		events.shellCalls = map[string]claudeShellCall{}
	}
	events.shellCalls[id] = claudeShellCall{command: bash.Command, started: when, async: bash.Background}
}

func (events *claudeWorkerEvents) recordShellResult(call claudeShellCall, result json.RawMessage, when time.Time) {
	var value struct {
		TaskID string `json:"backgroundTaskId"`
	}
	if call.async && json.Unmarshal(result, &value) == nil && value.TaskID != "" {
		call.answered = when
		events.backgroundShells = append(events.backgroundShells, call)
	}
}

const shellTimeMargin = 5 * time.Second

type shellEvidence struct {
	owner      string
	command    string
	background bool
	pending    bool
}

// Darwin ps displays tabs and newlines as octal escapes and keeps literal backslashes.
var darwinShellWhitespace = strings.NewReplacer("\t", `\011`, "\n", `\012`)

func matchesShellCommand(displayed, recorded string) bool {
	return displayed == recorded || (runtime.GOOS == "darwin" &&
		strings.ContainsAny(recorded, "\t\n") && displayed == darwinShellWhitespace.Replace(recorded))
}

func (s *Scanner) shellEvidence(file, command string, since time.Time) shellEvidence {
	evidence := shellEvidence{command: command}
	kept := s.claudeWorkers[file]
	if kept == nil {
		return evidence
	}
	matches := 0
	for path, log := range kept.logs {
		if log == nil {
			continue
		}
		for _, call := range log.value.shellCalls {
			if since.Sub(call.started) >= -time.Second && matchesShellCommand(command, call.command) {
				evidence.pending = true
			}
		}
		for _, call := range log.value.backgroundShells {
			startDelay, resultDelay := since.Sub(call.started), call.answered.Sub(since)
			if startDelay < -time.Second || startDelay > shellTimeMargin || resultDelay < 0 ||
				resultDelay > shellTimeMargin || !matchesShellCommand(command, call.command) {
				continue
			}
			matches++
			if matches == 1 {
				evidence.owner = path
				evidence.command = call.command
			} else if evidence.command != call.command {
				evidence.command = ""
			}
		}
	}
	if matches != 1 || evidence.owner == file {
		evidence.owner = ""
	}
	evidence.background = matches > 0
	return evidence
}

func shellGroup(parent, child ProcessInfo) bool {
	return parent.PID > 1 && parent.PGID > 1 && child.PID > 1 && child.PID != parent.PID &&
		child.PPID == parent.PID && child.PGID == child.PID && child.PGID != parent.PGID
}

func (s *Scanner) showStuckShells(rows []Conversation, after time.Duration, readTable func() ([]ProcessInfo, error)) {
	if !canStopShells() || after <= 0 || !slices.ContainsFunc(rows, func(c Conversation) bool { return c.shellSession != nil }) {
		return
	}
	table, err := readTable()
	if err != nil {
		return
	}
	byPID := make(map[int]ProcessInfo, len(table))
	children := make(map[int][]ProcessInfo)
	groupCount := make(map[int]int)
	for _, process := range table {
		if processIsZombie(process) {
			continue
		}
		byPID[process.PID] = process
		if process.PGID == process.PID {
			children[process.PPID] = append(children[process.PPID], process)
		}
		groupCount[process.PGID]++
	}
	cutoff := s.Now().Add(-after)
	var pids []int
	for _, row := range rows {
		if row.shellSession == nil {
			continue
		}
		parent := byPID[row.shellSession.PID]
		if !sameShellSession(parent, *row.shellSession) {
			continue
		}
		for _, child := range children[parent.PID] {
			if since, err := time.Parse(lstartLayout, child.Started); err == nil && !since.After(cutoff) && shellGroup(parent, child) {
				pids = append(pids, child.PID)
			}
		}
	}
	if len(pids) == 0 {
		return
	}
	args := s.Processes.Args(pids)
	for i := range rows {
		c := &rows[i]
		if c.shellSession != nil {
			parent := byPID[c.shellSession.PID]
			if sameShellSession(parent, *c.shellSession) {
				s.showShellGroups(c, parent, children[parent.PID], args, groupCount, cutoff)
			}
		}
	}
}

func sameShellSession(parent ProcessInfo, expected liveProcess) bool {
	return !processIsZombie(parent) && parent.PID == expected.PID && parent.Started != "" && parent.Started == strings.TrimSpace(expected.ProcStart)
}

func (s *Scanner) showShellGroups(c *Conversation, parent ProcessInfo, children []ProcessInfo, args map[int]string, counts map[int]int, cutoff time.Time) {
	if c.WorkingSubagents == nil {
		return
	}
	otherWork := c.mainWorking || len(c.Helpers) > 0 || *c.WorkingSubagents > 0
	for _, child := range children {
		if !shellGroup(parent, child) {
			continue
		}
		since, err := time.Parse(lstartLayout, child.Started)
		if err != nil || since.After(cutoff) {
			otherWork = true
			continue
		}
		command := shellCommand(args[child.PID])
		if command == "" {
			continue
		}
		evidence := s.shellEvidence(c.file, command, since)
		if evidence.pending || (c.mainWorking && !evidence.background) {
			otherWork = true
			continue
		}
		shell := StuckShell{
			PID: child.PID, ProcStart: child.Started, Since: stamp(since),
			Command: s.shown.get(texts{lastPrompt: evidence.command}).prompt, ProcessCount: counts[child.PGID],
		}
		if id, ok := strings.CutPrefix(filepath.Base(evidence.owner), "agent-"); ok {
			id = strings.TrimSuffix(id, ".jsonl")
			if ValidAgentID(id) {
				meta := s.claudeAgentMetaOf(strings.TrimSuffix(evidence.owner, ".jsonl") + ".meta.json")
				shell.HelperID = s.helperTitle(id, "")
				shell.HelperName = s.helperTitle(firstNonEmpty(meta.AgentType, meta.Description), "")
			}
		}
		c.StuckShells = append(c.StuckShells, shell)
	}
	slices.SortStableFunc(c.StuckShells, func(a, b StuckShell) int {
		if byAge := strings.Compare(a.Since, b.Since); byAge != 0 {
			return byAge
		}
		return a.PID - b.PID
	})
	if len(c.StuckShells) > 0 && !otherWork {
		c.shellOnly = true
		c.Waiting = &waitingOn{For: waitShell}
	}
}

func processIsZombie(process ProcessInfo) bool {
	return strings.HasPrefix(process.Stat, "Z")
}
