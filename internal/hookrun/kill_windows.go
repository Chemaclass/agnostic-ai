package hookrun

import "os/exec"

// killTree keeps the default cancel on Windows, which kills the hook
// process; a child it started is left to WaitDelay.
func killTree(*exec.Cmd) {}
