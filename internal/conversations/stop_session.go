package conversations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrSessionRefused  = errors.New("this chat no longer runs in a terminal here")
	ErrSessionStopping = errors.New("this chat is already being stopped")
)

// StopSessionReply says how a running Claude Code session ended.
// Forced means a force-stop signal was sent. StillRunning means the
// same process was still running at the last check.
type StopSessionReply struct {
	Forced       bool `json:"forced,omitempty"`
	StillRunning bool `json:"stillRunning,omitempty"`
}

const sessionGracePeriod = 5 * time.Second

// StopSession ends the confirmed Claude Code process in its terminal.
func (s *Scanner) StopSession(ctx context.Context, tool Tool, id string, target ProcessRef, opts ScanOptions) (StopSessionReply, error) {
	if !canStopShells() || tool != ToolClaude || !sessionIDPattern.MatchString(id) ||
		target.PID < 2 || target.PID == os.Getpid() || strings.TrimSpace(target.ProcStart) == "" {
		return StopSessionReply{}, ErrSessionRefused
	}
	if _, stopping := s.stoppingSessions.LoadOrStore(id, struct{}{}); stopping {
		return StopSessionReply{}, ErrSessionStopping
	}
	defer s.stoppingSessions.Delete(id)

	found, err := s.sessionToStop(ctx, id, target, opts)
	if err != nil {
		return StopSessionReply{}, fmt.Errorf("check running chat: %w", err)
	}
	if !found {
		return StopSessionReply{}, ErrSessionRefused
	}
	table, err := s.Processes.Table()
	if err != nil {
		return StopSessionReply{}, fmt.Errorf("check running chat: %w", err)
	}
	process, running := sessionProcessAs(table, target)
	if !running || process.TTY == "" || !s.holdsShellSession(id, liveProcess(target)) || serverProcessIDs(table)[target.PID] {
		return StopSessionReply{}, ErrSessionRefused
	}
	if err := ctx.Err(); err != nil {
		return StopSessionReply{}, err
	}
	signal := s.sessionSignal
	if signal == nil {
		signal = signalSession
	}
	if err := signal(target.PID, false); err != nil {
		return StopSessionReply{}, fmt.Errorf("stop chat: %w", err)
	}
	s.shellStops.Add(1)
	wait := s.shellWait
	if wait == nil {
		wait = waitForShell
	}
	if err := wait(context.WithoutCancel(ctx), sessionGracePeriod); err != nil {
		return StopSessionReply{}, err
	}
	// After the signal only the process table decides: Claude Code may
	// remove its session file while it still shuts down.
	if table, err = s.Processes.Table(); err != nil {
		return StopSessionReply{}, fmt.Errorf("check chat after stopping: %w", err)
	}
	current, running := sessionProcessAs(table, target)
	if !running {
		return StopSessionReply{}, nil
	}
	if current.TTY != process.TTY || serverProcessIDs(table)[target.PID] {
		return StopSessionReply{StillRunning: true}, nil
	}
	if err := signal(target.PID, true); err != nil {
		return StopSessionReply{}, fmt.Errorf("force stop chat: %w", err)
	}
	s.shellStops.Add(1)
	if err := wait(context.WithoutCancel(ctx), time.Second); err != nil {
		return StopSessionReply{Forced: true}, err
	}
	if table, err = s.Processes.Table(); err != nil {
		return StopSessionReply{Forced: true}, fmt.Errorf("check chat after force stopping: %w", err)
	}
	_, running = sessionProcessAs(table, target)
	return StopSessionReply{Forced: true, StillRunning: running}, nil
}

func (s *Scanner) sessionToStop(ctx context.Context, id string, target ProcessRef, opts ScanOptions) (bool, error) {
	s.lock()
	defer s.unlock()
	result, err := s.scanClaude(ctx, opts)
	if err != nil {
		return false, err
	}
	for _, c := range result.conversations {
		if confirmed, ok := terminalSessionCandidate(c); c.ID == id && ok && confirmed == target {
			return true, nil
		}
	}
	return false, nil
}

func terminalSessionCandidate(c Conversation) (ProcessRef, bool) {
	if c.Tool != ToolClaude || c.App != AppCLI || !c.Live || c.process == nil || c.process.PID < 2 || c.process.PID == os.Getpid() {
		return ProcessRef{}, false
	}
	target := ProcessRef{PID: c.process.PID, ProcStart: c.process.ProcStart}
	return target, strings.TrimSpace(target.ProcStart) != ""
}

func showStoppableSessions(rows []Conversation, readTable func() ([]ProcessInfo, error)) {
	if !canStopShells() {
		return
	}
	var table []ProcessInfo
	var serverPIDs map[int]bool
	for i := range rows {
		target, ok := terminalSessionCandidate(rows[i])
		if !ok {
			continue
		}
		if table == nil {
			var err error
			table, err = readTable()
			if err != nil || len(table) == 0 {
				return
			}
			serverPIDs = serverProcessIDs(table)
		}
		if process, running := sessionProcessAs(table, target); running && process.TTY != "" && !serverPIDs[target.PID] {
			rows[i].Stoppable = &target
		}
	}
}

func sessionProcessAs(table []ProcessInfo, target ProcessRef) (ProcessInfo, bool) {
	for _, info := range table {
		if info.PID == target.PID {
			return info, sameShellSession(info, liveProcess(target))
		}
	}
	return ProcessInfo{}, false
}

// serverProcessIDs includes the server and ancestors that must stay alive.
func serverProcessIDs(table []ProcessInfo) map[int]bool {
	parents := make(map[int]int, len(table))
	for _, info := range table {
		parents[info.PID] = info.PPID
	}
	serverPIDs := map[int]bool{}
	for at := os.Getpid(); at > 1 && !serverPIDs[at]; at = parents[at] {
		serverPIDs[at] = true
	}
	return serverPIDs
}
