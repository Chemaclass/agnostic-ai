package conversations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

var ErrSessionRefused = errors.New("this chat no longer runs here")

// StopSessionReply says how a running Claude Code session ended.
// Forced: it ignored the request to stop and was killed. StillRunning:
// even that did not end it.
type StopSessionReply struct {
	Forced       bool `json:"forced,omitempty"`
	StillRunning bool `json:"stillRunning,omitempty"`
}

const sessionGracePeriod = 5 * time.Second

// StopSession ends the process that runs a live Claude Code session on
// this machine. It asks first and kills only a process that is still the
// same one after the grace period. The transcript stays, so the chat can
// be resumed.
func (s *Scanner) StopSession(ctx context.Context, tool Tool, id string) (StopSessionReply, error) {
	if !canStopShells() || tool != ToolClaude || !sessionIDPattern.MatchString(id) {
		return StopSessionReply{}, ErrSessionRefused
	}
	if _, stopping := s.stoppingSessions.LoadOrStore(id, struct{}{}); stopping {
		return StopSessionReply{}, ErrSessionRefused
	}
	defer s.stoppingSessions.Delete(id)

	process, running := s.sessionToStop(id)
	if !running {
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
		return StopSessionReply{}, fmt.Errorf("stop session: %w", err)
	}
	s.shellStops.Add(1)
	wait := s.shellWait
	if wait == nil {
		wait = waitForShell
	}
	if err := wait(context.WithoutCancel(ctx), sessionGracePeriod); err != nil {
		return StopSessionReply{}, err
	}
	if !s.runsSession(id, process) {
		return StopSessionReply{}, nil
	}
	if err := signal(process.PID, true); err != nil {
		return StopSessionReply{}, fmt.Errorf("force stop session: %w", err)
	}
	s.shellStops.Add(1)
	if err := wait(context.WithoutCancel(ctx), time.Second); err != nil {
		return StopSessionReply{Forced: true}, err
	}
	return StopSessionReply{Forced: true, StillRunning: s.runsSession(id, process)}, nil
}

func (s *Scanner) sessionToStop(id string) (liveProcess, bool) {
	s.lock()
	live, _, _ := s.claudeLiveSessions()
	s.unlock()
	session, found := live[id]
	if !found || session.process.PID <= 1 || session.process.PID == os.Getpid() {
		return liveProcess{}, false
	}
	return session.process, s.runsSession(id, session.process)
}

// runsSession: the process table still shows the session's own process,
// started when its session file says, and that file still names the session.
func (s *Scanner) runsSession(id string, process liveProcess) bool {
	table, err := s.Processes.Table()
	if err != nil {
		return false
	}
	for _, info := range table {
		if info.PID == process.PID {
			return sameShellSession(info, process) && s.holdsShellSession(id, process)
		}
	}
	return false
}
