//go:build !unix && !windows

package mcpprobe

import "testing"

func processRunning(t *testing.T, _ int) bool {
	t.Helper()
	t.Skip("owned MCP processes are unsupported on this platform")
	return false
}
