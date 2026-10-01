//go:build unix

package hookrun

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRun_KillsABackgroundChildOnceTheHookExits(t *testing.T) {
	r := Run([]string{"sh", "-c", "sleep 600 >/dev/null 2>&1 & echo $!"}, t.TempDir(), os.Environ(), nil, time.Minute)
	pid, err := strconv.Atoi(strings.TrimSpace(r.Stdout))
	if err != nil {
		t.Fatalf("no child pid in %+v", r)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("background child %d outlived the run", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRun_AnExitBeforeTheTimeoutIsNoTimeoutWhenAChildHoldsThePipes(t *testing.T) {
	r := Run([]string{"sh", "-c", "sleep 3 & exit 0"}, t.TempDir(), os.Environ(), nil, time.Second)
	if r.TimedOut || r.Exit != 0 || r.StartErr != nil {
		t.Errorf("result = %+v, want exit 0 and no timeout", r)
	}
}
