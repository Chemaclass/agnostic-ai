//go:build !unix && !windows

package mcpprobe

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

func headersCommand(ctx context.Context, helper string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", helper)
}

func startProcess(context.Context, *exec.Cmd) (func() error, error) {
	return nil, fmt.Errorf("owned MCP processes are unsupported on %s", runtime.GOOS)
}
