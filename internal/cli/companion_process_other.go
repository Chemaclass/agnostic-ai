//go:build !windows

package cli

func companionProcessRuns(pid int) bool {
	return pidAlive(pid)
}
