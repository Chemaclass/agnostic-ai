package cli

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
)

// companionHoldTimes is how long the hook waits for a click, per tool.
// Claude Code shows its prompt while the hook runs, so the terminal
// answer is never delayed (decision 028). Codex shows its prompt only
// after the hook ends, so its wait is short (decision 029). The hook
// entry's timeout must outlast it.
var companionHoldTimes = map[string]time.Duration{
	"claude": 300 * time.Second,
	"codex":  60 * time.Second,
}

// companionPoll is how often a held hook reads its decision file.
var companionPoll = 200 * time.Millisecond

// companionHold is a request the gate left to the person, which the
// hook may hold for an answer from the companion.
type companionHold struct {
	target, source, mode string
	payload              approvePayload
	rawInput             json.RawMessage
	request              approvals.Request
	log                  approvals.Log
	// on is approvals.companion, with the tool's settings read cleanly.
	on bool
	// reviewer is set when sync turned on the tool's own reviewer, which
	// it does from the home config alone.
	reviewer bool
}

// wait holds the request until the companion answers or the hold time
// passes, and returns the reply to print: the allow or deny reply after
// a click with a matching digest, nil in every other case.
func (h companionHold) wait(now time.Time) []byte {
	holdFor, held := companionHoldTimes[h.target]
	if !held || !h.on || len(h.rawInput) == 0 {
		return nil
	}
	if h.target == "codex" && h.reviewer {
		// Codex runs the hook before its reviewer: a hold delays it.
		return nil
	}
	if !approvals.CompanionLive(approvals.LivePath(h.source), now) {
		return nil
	}
	input, ok := approvals.ShowableInput(h.payload.Call, h.payload.Input)
	if !ok {
		return nil
	}
	id, err := approvals.NewPendingID()
	if err != nil {
		return nil
	}
	dir := approvals.PendingDir(h.source)
	digest := approvals.PendingDigest(h.target, h.payload.Session, h.payload.Call, h.rawInput)
	record := approvals.Pending{
		ID: id, Tool: h.target, Session: h.payload.Session, Cwd: h.payload.Cwd, Call: h.payload.Call, Input: input,
		Digest: digest, Created: now.UTC(), Expires: now.Add(holdFor).UTC(), PID: os.Getpid(),
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(stop)
	defer approvals.RemovePending(dir, id)
	if err := approvals.WritePending(dir, record); err != nil {
		return nil
	}
	decision, answered := awaitCompanion(dir, id, digest, time.Now().Add(holdFor), stop)
	outcome := approvals.Decision{Outcome: approvals.Asked, Rule: approvals.RuleCompanion}
	var reply []byte
	switch decision {
	case approvals.DecideApprove:
		outcome = approvals.Decision{Outcome: approvals.Approved}
		reply, _ = hookrun.PermissionAllowReply(h.target)
	case approvals.DecideDeny:
		outcome = approvals.Decision{Outcome: approvals.Denied}
		reply, _ = hookrun.PermissionDenyReply(h.target)
	}
	if !answered {
		reply = nil
	}
	entry := approvals.NewEntry(h.target, h.payload.Session, h.mode, h.request, outcome, time.Now())
	entry.By = approvals.ByCompanion
	if err := h.log.Append(entry); err != nil {
		// Every answer from the companion leaves a line; without one, ask.
		return nil
	}
	return reply
}

// awaitCompanion reads the decision file for id until one with digest
// comes, the deadline passes, or a signal stops the hook. answered is
// false for a timeout, a signal or a decision for another digest.
func awaitCompanion(dir, id, digest string, deadline time.Time, stop <-chan os.Signal) (decision string, answered bool) {
	tick := time.NewTicker(companionPoll)
	defer tick.Stop()
	for {
		if d, ok := approvals.ReadDecision(dir, id); ok {
			if d.Digest != digest {
				return "", false
			}
			return d.Decision, d.Decision != approvals.DecideRelease
		}
		if !time.Now().Before(deadline) {
			return "", false
		}
		select {
		case <-stop:
			return "", false
		case <-tick.C:
		}
	}
}
