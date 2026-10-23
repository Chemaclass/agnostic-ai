//go:build unix

package mcpprobe

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func processRunning(t *testing.T, pid int) bool {
	t.Helper()
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false
		}
		t.Fatalf("check owned descendant: %v", err)
	}
	state := strings.TrimSpace(string(out))
	return state != "" && !strings.HasPrefix(state, "Z")
}
