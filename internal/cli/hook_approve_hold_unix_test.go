//go:build !windows

package cli

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestHookApproveHold_SignalRemovesTheRecordAndStaysSilent(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			source, project := holdSetup(t, companionAskHome, 10*time.Second)
			out := runHeld(t, "claude", recordedPayload(t, "claude", project, "touch a.txt"))
			heldRecord(t, source)
			// The hook catches the signal while it holds, so the test process lives on.
			if err := syscall.Kill(os.Getpid(), sig); err != nil {
				t.Fatal(err)
			}
			if got := reply(t, out); got != "" {
				t.Fatalf("reply = %q, want nothing after %s", got, sig)
			}
			assertNoPending(t, source)
			entries := approveLog(t, source)
			if len(entries) != 1 || entries[0].Decision != approvals.Asked || entries[0].Rule != approvals.RuleCompanion {
				t.Errorf("log = %+v", entries)
			}
		})
	}
}
