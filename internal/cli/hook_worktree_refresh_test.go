package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHookWorktreeRefresh_CancelsReadyDescendant(t *testing.T) {
	main, linked, _, marker := refreshFixture(t, false)
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("AGNOSTIC_REFRESH_TEST_EXECUTABLE", filepath.ToSlash(helper))
	t.Setenv("AGNOSTIC_REFRESH_DESCENDANT_READY", ready)
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\nsetup: |\n  \"$AGNOSTIC_REFRESH_TEST_EXECUTABLE\" -test.run=^TestHookWorktreeRefreshDescendant$\n")
	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync descendant setup: %v\n%s", err, out)
	}
	mustWriteFile(t, marker, "completed before refresh")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"hook", "worktree-refresh", "--repo", main, "--worktree", linked})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	finished := false
	defer func() {
		cancel()
		if !finished {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("refresh did not stop during test cleanup")
			}
		}
	}()
	var address string
	deadline := time.Now().Add(10 * time.Second)
	for address == "" {
		if raw, err := os.ReadFile(ready); err == nil {
			address = strings.TrimSpace(string(raw))
		}
		if address != "" {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("setup descendant never became ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	request := func(message string) (string, error) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			return "", err
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			return "", err
		}
		if _, err := conn.Write([]byte(message + "\n")); err != nil {
			return "", err
		}
		return bufio.NewReader(conn).ReadString('\n')
	}
	defer func() { _, _ = request("stop") }()
	if reply, err := request("probe"); err != nil || reply != "alive\n" {
		t.Fatalf("descendant readiness = %q, %v", reply, err)
	}
	cancel()
	select {
	case err := <-done:
		finished = true
		if !errors.Is(err, context.Canceled) {
			t.Errorf("refresh cancellation = %v\n%s", err, &out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled refresh did not return")
	}
	if reply, err := request("probe"); err == nil && reply == "alive\n" {
		t.Error("setup descendant remains alive after refresh cancellation returned")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("canceled refresh retained completion marker: %v", err)
	}
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\nsetup: printf 'recovered\\n' > recovered-setup\n")
	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync recovery setup: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err != nil {
		t.Fatalf("refresh after cancellation: %v\n%s", err, out)
	}
	if got, err := os.ReadFile(filepath.Join(linked, "recovered-setup")); err != nil || string(got) != "recovered\n" {
		t.Errorf("recovery setup = %q, %v", got, err)
	}
}

func TestHookWorktreeRefreshDescendant(t *testing.T) {
	ready := os.Getenv("AGNOSTIC_REFRESH_DESCENDANT_READY")
	if ready == "" {
		return
	}
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := listener.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ready, []byte(listener.Addr().String()), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		message, _ := bufio.NewReader(conn).ReadString('\n')
		if message == "stop\n" {
			_ = conn.Close()
			return
		}
		_, _ = conn.Write([]byte("alive\n"))
		_ = conn.Close()
	}
}

func TestHookWorktreeRefresh_KeepsSuccessfulBackgroundSetup(t *testing.T) {
	main, linked, _, _ := refreshFixture(t, false)
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("AGNOSTIC_REFRESH_TEST_EXECUTABLE", filepath.ToSlash(helper))
	t.Setenv("AGNOSTIC_REFRESH_DESCENDANT_READY", ready)
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\nsetup: |\n  \"$AGNOSTIC_REFRESH_TEST_EXECUTABLE\" -test.run=^TestHookWorktreeRefreshDescendant$ > descendant-output 2>&1 &\n")
	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync background setup: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err != nil {
		t.Fatalf("background setup: %v\n%s", err, out)
	}
	var address string
	deadline := time.Now().Add(10 * time.Second)
	for address == "" {
		if raw, err := os.ReadFile(ready); err == nil {
			address = strings.TrimSpace(string(raw))
		}
		if address != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("successful background setup did not remain alive")
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("successful background setup was stopped: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("probe\n")); err != nil {
		t.Fatal(err)
	}
	if reply, err := bufio.NewReader(conn).ReadString('\n'); err != nil || reply != "alive\n" {
		t.Errorf("successful background setup = %q, %v", reply, err)
	}
	stop, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		_, _ = stop.Write([]byte("stop\n"))
		_ = stop.Close()
	}
}

func refreshFixture(t *testing.T, override bool) (main, linked, script, marker string) {
	t.Helper()
	main = setupGitRepo(t)
	config := "version: 1\ntargets: [claude]\n"
	dir := ".claude"
	if override {
		dir = "tool output/claude"
		config += "outputs:\n  claude:\n    dir: '" + dir + "'\n"
	}
	mustWriteFile(t, filepath.Join(main, "agnostic-ai.yaml"), config)
	mustWriteFile(t, filepath.Join(main, ".agnostic-ai", "environments", "dev.yaml"), "name: dev\nsetup: |\n  printf 'setup\\n' >> setup-runs\n  test ! -e fail-setup\n")
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "setup fixture")
	linked = filepath.Join(t.TempDir(), "linked worktree with spaces")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	testutil.Chdir(t, linked)
	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync setup: %v\n%s", err, out)
	}
	script = filepath.Join(dir, "hooks", "agnostic-ai-worktree-setup.sh")
	gitDir := strings.TrimSpace(git(t, linked, "rev-parse", "--absolute-git-dir"))
	marker = filepath.Join(gitDir, "agnostic-ai-worktree-setup")
	return
}

func nativeRefreshSetup(t *testing.T, linked, script string) error {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"cwd": filepath.ToSlash(linked)})
	if err != nil {
		t.Fatal(err)
	}
	argv := hookrun.Argv("claude", runtime.GOOS, hookrun.Handler{Shell: "bash"})
	if runtime.GOOS == "windows" && !filepath.IsAbs(argv[0]) {
		t.Fatal("native setup acceptance requires Git Bash, not a PATH bash launcher")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], filepath.ToSlash(script))
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Dir = linked
	cmd.Stdin = strings.NewReader(string(payload))
	cmd.Env = append(os.Environ(), "AGNOSTIC_AI_TARGET=claude")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("native setup: %s", out)
		return err
	}
	return nil
}

func TestHookWorktreeRefresh_RerunsConfiguredSetupOnly(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "output override"}[override], func(t *testing.T) {
			main, linked, script, marker := refreshFixture(t, override)
			other := filepath.Join(t.TempDir(), "other linked worktree")
			git(t, main, "worktree", "add", "-q", "--detach", other)
			otherMarker := filepath.Join(strings.TrimSpace(git(t, other, "rev-parse", "--absolute-git-dir")), "agnostic-ai-worktree-setup")
			mustWriteFile(t, otherMarker, "keep other marker")
			mainMarker := filepath.Join(main, ".git", "agnostic-ai-worktree-setup")
			mustWriteFile(t, mainMarker, "keep main marker")
			for range 2 {
				if err := nativeRefreshSetup(t, linked, script); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err != nil {
				t.Fatalf("refresh: %v\n%s", err, out)
			}
			if err := nativeRefreshSetup(t, linked, script); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(linked, "setup-runs")); err != nil || string(got) != "setup\nsetup\n" {
				t.Errorf("setup runs = %q, %v; want two", got, err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Errorf("successful refresh has no marker: %v", err)
			}
			for path, want := range map[string]string{otherMarker: "keep other marker", mainMarker: "keep main marker"} {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Errorf("unrelated marker changed: %s, %q, %v", path, got, err)
				}
			}
		})
	}
}

func TestHookWorktreeRefresh_FailureLeavesNoCompletion(t *testing.T) {
	main, linked, script, marker := refreshFixture(t, false)
	if err := nativeRefreshSetup(t, linked, script); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(linked, "fail-setup"), "fail")
	if _, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err == nil || !strings.Contains(err.Error(), "setup") {
		t.Errorf("failed setup must fail refresh with context: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("failed refresh kept completion: %v", err)
	}
	if err := os.Remove(filepath.Join(linked, "fail-setup")); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err != nil {
		t.Fatalf("retry: %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("recovered refresh has no marker: %v", err)
	}
}

func TestHookWorktreeRefresh_RejectsOtherCheckoutsWithoutReset(t *testing.T) {
	main, linked, _, marker := refreshFixture(t, false)
	mustWriteFile(t, marker, "keep")
	_, _, foreignLinked := disposableWorktree(t)
	for _, candidate := range []string{main, foreignLinked, filepath.Dir(linked)} {
		if _, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", candidate); err == nil {
			t.Errorf("accepted unrelated or main checkout: %s", candidate)
		}
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Errorf("rejection reset marker: %q, %v", got, err)
	}
}

func TestHookWorktreeRefresh_RequiresCurrentGeneratedSetup(t *testing.T) {
	for _, broken := range []string{"missing", "edited", "disabled"} {
		t.Run(broken, func(t *testing.T) {
			main, linked, script, marker := refreshFixture(t, false)
			mustWriteFile(t, marker, "keep")
			switch broken {
			case "missing":
				if err := os.Remove(script); err != nil {
					t.Fatal(err)
				}
			case "edited":
				mustWriteFile(t, script, "#!/bin/sh\nexit 0\n")
			case "disabled":
				mustWriteFile(t, filepath.Join(linked, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
			}
			if _, err := runCLI(t, "hook", "worktree-refresh", "--repo", main, "--worktree", linked); err == nil {
				t.Errorf("accepted %s setup", broken)
			}
			if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
				t.Errorf("invalid setup reset marker: %q, %v", got, err)
			}
		})
	}
}
