package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrShellRefused = errors.New("this shell is no longer eligible to stop")

type StopShellReply struct {
	StillRunning []ProcessRef `json:"stillRunning,omitempty"`
}

const shellGracePeriod = 5 * time.Second

func (s *Scanner) StopShell(ctx context.Context, tool Tool, id string, target ProcessRef, opts ScanOptions) (StopShellReply, error) {
	if !canStopShells() || opts.shellAfter() <= 0 || tool != ToolClaude || !sessionIDPattern.MatchString(id) ||
		target.PID <= 1 || target.PID == os.Getpid() || target.ProcStart == "" {
		return StopShellReply{}, ErrShellRefused
	}
	if _, stopping := s.stoppingShells.LoadOrStore(target, struct{}{}); stopping {
		return StopShellReply{}, ErrShellRefused
	}
	defer s.stoppingShells.Delete(target)

	parent, eligible := s.shellToStop(ctx, id, target, opts)
	if !eligible {
		return StopShellReply{}, ErrShellRefused
	}
	table, err := s.Processes.Table()
	if err != nil {
		return StopShellReply{}, fmt.Errorf("check running shell: %w", err)
	}
	if !s.holdsShellSession(id, parent) || !sameRunningShell(table, parent, target) {
		return StopShellReply{}, ErrShellRefused
	}
	if err := ctx.Err(); err != nil {
		return StopShellReply{}, err
	}
	tracked := shellDescendants(table, target.PID)
	signal := s.shellSignal
	if signal == nil {
		signal = signalShellGroup
	}
	if err := signal(target.PID, false); err != nil {
		return StopShellReply{}, fmt.Errorf("stop shell: %w", err)
	}
	s.shellStops.Add(1)
	wait := s.shellWait
	if wait == nil {
		wait = waitForShell
	}
	if err := wait(context.WithoutCancel(ctx), shellGracePeriod); err != nil {
		return StopShellReply{}, err
	}

	fresh, err := s.Processes.Table()
	if err != nil {
		return StopShellReply{}, fmt.Errorf("check shell after stopping: %w", err)
	}
	groupSignaled := s.holdsShellSession(id, parent) && sameRunningShell(fresh, parent, target)
	if groupSignaled {
		tracked = append(tracked, shellDescendants(fresh, target.PID)...)
		if err := signal(target.PID, true); err != nil {
			return StopShellReply{}, fmt.Errorf("force stop shell: %w", err)
		}
		s.shellStops.Add(1)
	}
	return StopShellReply{StillRunning: stillRunningShellProcesses(fresh, tracked, target.PID, groupSignaled)}, nil
}

func (s *Scanner) shellToStop(ctx context.Context, id string, target ProcessRef, opts ScanOptions) (liveProcess, bool) {
	s.lock()
	defer s.unlock()
	result, _ := s.scanClaude(ctx, opts)
	for _, c := range result.conversations {
		if c.ID != id || !c.Live || c.shellSession == nil {
			continue
		}
		rows := []Conversation{c}
		s.showStuckShells(rows, opts.shellAfter(), s.Processes.Table)
		if slices.ContainsFunc(rows[0].StuckShells, func(shell StuckShell) bool {
			return shell.PID == target.PID && shell.ProcStart == target.ProcStart
		}) {
			return *c.shellSession, true
		}
	}
	return liveProcess{}, false
}

func (s *Scanner) holdsShellSession(id string, process liveProcess) bool {
	path := filepath.Join(s.Stores.ClaudeDir, "sessions", strconv.Itoa(process.PID)+".json")
	raw, err := readClaudeMetadata(path)
	var entry claudeLive
	return err == nil && json.Unmarshal(raw, &entry) == nil && entry.SessionID == id &&
		entry.PID == process.PID && strings.TrimSpace(entry.ProcStart) == strings.TrimSpace(process.ProcStart)
}

func sameRunningShell(table []ProcessInfo, parent liveProcess, target ProcessRef) bool {
	var session, leader ProcessInfo
	for _, process := range table {
		switch process.PID {
		case parent.PID:
			session = process
		case target.PID:
			leader = process
		}
	}
	return sameShellSession(session, parent) && leader.Started == target.ProcStart && shellGroup(session, leader)
}

func shellDescendants(table []ProcessInfo, group int) []ProcessRef {
	children := make(map[int][]ProcessInfo)
	var queue []ProcessInfo
	for _, process := range table {
		children[process.PPID] = append(children[process.PPID], process)
		if process.PGID == group {
			queue = append(queue, process)
		}
	}
	var tracked []ProcessRef
	seen := make(map[int]bool)
	for len(queue) > 0 {
		process := queue[0]
		queue = queue[1:]
		if seen[process.PID] {
			continue
		}
		seen[process.PID] = true
		if process.Started != "" {
			tracked = append(tracked, ProcessRef{PID: process.PID, ProcStart: process.Started})
		}
		queue = append(queue, children[process.PID]...)
	}
	return tracked
}

func stillRunningShellProcesses(table []ProcessInfo, tracked []ProcessRef, group int, groupSignaled bool) []ProcessRef {
	original := make(map[ProcessRef]bool, len(tracked))
	for _, process := range tracked {
		original[process] = true
	}
	var running []ProcessRef
	for _, process := range table {
		ref := ProcessRef{PID: process.PID, ProcStart: process.Started}
		if original[ref] && !processIsZombie(process) && (!groupSignaled || process.PGID != group) {
			running = append(running, ref)
		}
	}
	slices.SortFunc(running, func(a, b ProcessRef) int { return a.PID - b.PID })
	return running
}

func waitForShell(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
