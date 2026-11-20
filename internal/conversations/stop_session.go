package conversations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

var (
	ErrSessionRefused  = errors.New("this chat no longer runs in a terminal here")
	ErrSessionStopping = errors.New("this chat is already being stopped")
)

// StopSessionReply says how a running Claude Code session ended.
// Forced: it ignored the request to stop and was killed. StillRunning:
// even that did not end it.
type StopSessionReply struct {
	Forced       bool `json:"forced,omitempty"`
	StillRunning bool `json:"stillRunning,omitempty"`
}

const sessionGracePeriod = 5 * time.Second

// StopSession ends the process of a live Claude Code session that runs in
// a terminal on this machine. It asks first and kills only a process that
// is still the same one after the grace period.
func (s *Scanner) StopSession(ctx context.Context, tool Tool, id string, opts ScanOptions) (StopSessionReply, error) {
	if !canStopShells() || tool != ToolClaude || !sessionIDPattern.MatchString(id) {
		return StopSessionReply{}, ErrSessionRefused
	}
	if _, stopping := s.stoppingSessions.LoadOrStore(id, struct{}{}); stopping {
		return StopSessionReply{}, ErrSessionStopping
	}
	defer s.stoppingSessions.Delete(id)

	process, found := s.sessionToStop(ctx, id, opts)
	if !found {
		return StopSessionReply{}, ErrSessionRefused
	}
	table, err := s.Processes.Table()
	if err != nil {
		return StopSessionReply{}, fmt.Errorf("check running chat: %w", err)
	}
	if !runningAs(table, process) || !s.holdsShellSession(id, process) || holdsServer(table, process.PID) {
		return StopSessionReply{}, ErrSessionRefused
	}
	if err := ctx.Err(); err != nil {
		return StopSessionReply{}, err
	}
	signal := s.sessionSignal
	if signal == nil {
		signal = signalSession
	}
	if err := signal(process.PID, false); err != nil {
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
	if !runningAs(table, process) {
		return StopSessionReply{}, nil
	}
	if err := signal(process.PID, true); err != nil {
		return StopSessionReply{}, fmt.Errorf("force stop chat: %w", err)
	}
	s.shellStops.Add(1)
	if err := wait(context.WithoutCancel(ctx), time.Second); err != nil {
		return StopSessionReply{Forced: true}, err
	}
	if table, err = s.Processes.Table(); err != nil {
		return StopSessionReply{Forced: true}, fmt.Errorf("check chat after force stopping: %w", err)
	}
	return StopSessionReply{Forced: true, StillRunning: runningAs(table, process)}, nil
}

// sessionToStop finds the row as a scan lists it, so the endpoint stops
// only what the list offers to stop.
func (s *Scanner) sessionToStop(ctx context.Context, id string, opts ScanOptions) (liveProcess, bool) {
	s.lock()
	defer s.unlock()
	for _, c := range s.scanClaude(ctx, opts).conversations {
		if c.ID == id && c.Stoppable && c.process != nil && c.process.PID > 1 && c.process.PID != os.Getpid() {
			return *c.process, true
		}
	}
	return liveProcess{}, false
}

func runningAs(table []ProcessInfo, process liveProcess) bool {
	for _, info := range table {
		if info.PID == process.PID {
			return sameShellSession(info, process)
		}
	}
	return false
}

// holdsServer: pid is this server or one of its ancestors, such as a chat
// that started the companion from its shell. Ending it would end the server.
func holdsServer(table []ProcessInfo, pid int) bool {
	parents := make(map[int]int, len(table))
	for _, info := range table {
		parents[info.PID] = info.PPID
	}
	for at, steps := os.Getpid(), 0; at > 1 && steps < len(table)+1; at, steps = parents[at], steps+1 {
		if at == pid {
			return true
		}
	}
	return false
}
