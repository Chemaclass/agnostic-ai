//go:build !unix

package conversations

import "os/exec"

// ownCodexTree keeps the default cancel off Unix, which kills the
// app-server; a program it started is left to WaitDelay and to the
// closed output.
func ownCodexTree(*exec.Cmd) {}

// reapCodexTree leaves programs the app-server started alone off Unix,
// which has no process group to kill.
func reapCodexTree(*exec.Cmd) {}
