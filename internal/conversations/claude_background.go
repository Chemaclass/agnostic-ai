package conversations

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// claudeShortIDPattern is the form of the id claude attach takes.
var claudeShortIDPattern = regexp.MustCompile(`^[0-9a-f]{1,8}$`)

// openClaudeBackground shows a Claude Code session started with claude
// --bg. Its terminal belongs to Claude's daemon, so no app shows it; a
// terminal shows it while claude attach runs there. A terminal that
// already runs claude attach on it comes to the front; with none, a new
// tab runs claude attach. claude attach starts no second copy: the
// session keeps running when its tab closes.
func openClaudeBackground(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	pid := c.process.PID
	jobs, err := processes.ClaudeJobs(ctx)
	if err != nil {
		return OpenReply{Message: fmt.Sprintf("It runs as a Claude Code background session (process %d), but the list of those sessions could not be read: %v. Run claude agents to find its id, then claude attach <id>.", pid, err)}
	}
	job, listed := jobOfSession(jobs, c.ID)
	if !listed {
		return OpenReply{Message: fmt.Sprintf("It runs as a Claude Code background session (process %d), but claude agents does not list it, so it cannot be attached from here. Run claude agents to find its id, then claude attach <id>.", pid)}
	}
	command := attachCommand(c.Folder, job.ShortID)
	table, err := processes.Table()
	if err != nil {
		return OpenReply{Message: fmt.Sprintf("It runs as a Claude Code background session, and the terminals that show it could not be looked for (%v). Run this command in a terminal.", err), Command: command}
	}
	if viewer, ok := attachedViewer(table, processes, job); ok {
		reply := switchToHost(ctx, viewerFolder(viewer, processes), viewer, launcher, "")
		// claude attach starts no second copy, so a switch that did not
		// show the session still offers it.
		if !reply.Opened {
			reply.Command = command
		}
		return reply
	}
	app := ""
	if c.lastTab != nil {
		app = c.lastTab.App
	}
	opened := openInTab(ctx, launcher, app, command)
	switch {
	case opened.err == nil && opened.window:
		return OpenReply{Opened: true, Message: "Opened it in a new Terminal window with claude attach."}
	case opened.err == nil:
		return OpenReply{Opened: true, Message: fmt.Sprintf("Opened it in a new %s tab with claude attach.", opened.app)}
	default:
		return notOpenedInTab(opened, command)
	}
}

// viewerFolder is the folder an editor's attach client runs in, which
// names the editor window that shows it; the session's own folder may
// be another project. It is empty for any other app, and when the
// folder cannot be read, so no window is guessed.
func viewerFolder(viewer placement, processes Processes) string {
	if _, editor := editorTerminals[viewer.host.App]; !editor {
		return ""
	}
	pid := viewer.process.PID
	return processes.Cwds([]int{pid})[pid]
}

// jobOfSession finds the background session whose transcript id is sessionID.
func jobOfSession(jobs []ClaudeJob, sessionID string) (ClaudeJob, bool) {
	for _, job := range jobs {
		if job.SessionID == sessionID && claudeShortIDPattern.MatchString(job.ShortID) {
			return job, true
		}
	}
	return ClaudeJob{}, false
}

// attachCommand runs claude attach in folder. The session does not
// need the folder, which only sets where the tab's shell is left, so a
// folder with control characters is left out.
func attachCommand(folder, shortID string) string {
	attach := "claude attach " + shortID
	if folder == "" || hasControl(folder) {
		return attach
	}
	return "cd " + shellQuote(folder) + " && " + attach
}

// attachedViewer finds a terminal that already shows job: a claude
// attach process whose argument is the job's short id or its exact
// name. claude attach also takes the start of an id or part of a name;
// those are not matched, since they may name another session.
// Of several, one in a tab app wins, then the latest started.
func attachedViewer(table []ProcessInfo, processes Processes, job ClaudeJob) (placement, bool) {
	var clients []ProcessInfo
	var pids []int
	for _, process := range table {
		if process.TTY != "" && filepath.Base(process.Command) == "claude" {
			clients = append(clients, process)
			pids = append(pids, process.PID)
		}
	}
	if len(clients) == 0 {
		return placement{}, false
	}
	args := processes.Args(pids)
	var best placement
	found := false
	for _, client := range clients {
		target, attaches := claudeAttachTarget(args[client.PID])
		if !attaches || (target != job.ShortID && target != job.Name) {
			continue
		}
		host, ok := hostOf(table, client.PID)
		if !ok {
			continue
		}
		viewer := placement{process: liveProcess{PID: client.PID, ProcStart: client.Started}, host: host}
		if !found || betterViewer(viewer, best) {
			best, found = viewer, true
		}
	}
	return best, found
}

// betterViewer reports whether a should be shown rather than b.
func betterViewer(a, b placement) bool {
	if isTabApp(a.host.App) != isTabApp(b.host.App) {
		return isTabApp(a.host.App)
	}
	aStarted, aErr := time.Parse(lstartLayout, a.process.ProcStart)
	bStarted, bErr := time.Parse(lstartLayout, b.process.ProcStart)
	return aErr == nil && bErr == nil && aStarted.After(bStarted)
}

// claudeAttachTarget reads a claude attach command line, which ps
// prints without quotes, so a name with spaces comes back joined by
// single spaces.
func claudeAttachTarget(args string) (string, bool) {
	fields := strings.Fields(args)
	if len(fields) < 3 || filepath.Base(fields[0]) != "claude" || fields[1] != "attach" {
		return "", false
	}
	return strings.Join(fields[2:], " "), true
}
