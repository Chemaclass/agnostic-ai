//go:build !windows

package conversations

// nativeProcesses is nil here: ps and lsof read the processes.
var nativeProcesses nativeReader
