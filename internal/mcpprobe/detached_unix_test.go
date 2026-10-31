//go:build unix

package mcpprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestMCPProbeDetachedHelper(t *testing.T) {
	switch os.Getenv("MCPPROBE_DETACHED_MODE") {
	case "child":
		time.Sleep(30 * time.Second)
	case "server":
		child := exec.Command(os.Args[0], "-test.run=^TestMCPProbeDetachedHelper$")
		child.Env = append(os.Environ(), "MCPPROBE_DETACHED_MODE=child")
		child.Stderr = os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(os.Getenv("MCPPROBE_DETACHED_PID"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			os.Exit(2)
		}
		_ = child.Wait()
	}
}

func TestListTools_StdioTimeoutIgnoresDetachedStderr(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := ListTools(ctx, Server{Command: exe, Args: []string{"-test.run=^TestMCPProbeDetachedHelper$"}, Env: map[string]string{"MCPPROBE_DETACHED_MODE": "server", "MCPPROBE_DETACHED_PID": pidFile}}, "test")
		done <- err
	}()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("fake server did not start a detached child")
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Kill(); _ = child.Release() }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context deadline exceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed-out listing waited for a detached child holding stderr")
	}
}
