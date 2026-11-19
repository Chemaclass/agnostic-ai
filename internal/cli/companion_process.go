package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// errNoSessionBus is a Linux shell, such as an SSH login, with no D-Bus
// session bus, so a companion that runs in the desktop session is out of reach.
var errNoSessionBus = errors.New("this shell has no D-Bus session bus, so it cannot reach a companion")

// sessionBusMissing reports whether a program started here finds no
// D-Bus session bus: no address in the environment and none of the
// places the D-Bus library looks for one in the user's runtime folder.
func sessionBusMissing(getenv func(string) string, uid string, exists func(string) bool) bool {
	if getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return false
	}
	folders := []string{filepath.Join("/run/user", uid)}
	if runtimeDir := getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		folders = append(folders, runtimeDir)
	}
	for _, folder := range folders {
		if exists(filepath.Join(folder, "bus")) || exists(filepath.Join(folder, "dbus-session")) {
			return false
		}
	}
	return true
}

// runGoBuild runs `go args...` in dir and hands each line of its output to lines.
func runGoBuild(ctx context.Context, dir string, env []string, lines func(string), args ...string) error {
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = dir
	command.Env = env
	reader, writer := io.Pipe()
	command.Stdout, command.Stderr = writer, writer
	var read sync.WaitGroup
	read.Go(func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for scanner.Scan() {
			lines(scanner.Text())
		}
		// Whatever is left is thrown away so the command never blocks on a full pipe.
		_, _ = io.Copy(io.Discard, reader)
	})
	err := command.Run()
	_ = writer.Close()
	read.Wait()
	return err
}

func companionLaunchCommand(path, goos string) (command *exec.Cmd, viaOpen bool) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	if goos == "darwin" && filepath.Base(bundle) == companionAppName+".app" {
		// Each invocation starts a process so the existing companion receives its second-start event.
		return exec.Command("/usr/bin/open", "-g", "-n", "-a", bundle), true
	}
	return exec.Command(path), false
}

func startCompanion(path string, env []string) error {
	command, viaOpen := companionLaunchCommand(path, runtime.GOOS)
	command.Env = env
	if viaOpen {
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("open %s: %w", path, withWhatItSaid(err, output))
		}
		return nil
	}
	command.SysProcAttr = companionSysProcAttr()
	if err := command.Start(); err != nil {
		return err
	}
	// The companion outlives this command; nothing waits for it.
	return command.Process.Release()
}

// errCompanionCannotRun is a companion program that cannot run at all,
// as opposed to one too slow to answer.
var errCompanionCannotRun = errors.New("the companion cannot run")

func companionBuiltFor(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	command := exec.CommandContext(ctx, path, "--version")
	command.SysProcAttr = companionSysProcAttr()
	out, err := command.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(string(out)), nil
	case ctx.Err() != nil:
		return "", fmt.Errorf("ask the companion its version: %w", ctx.Err())
	case errors.As(err, &exit) && len(bytes.TrimSpace(out)) > 0:
		return "", fmt.Errorf("ask the companion its version: %w", err)
	default:
		return "", fmt.Errorf("%w: %w", errCompanionCannotRun, err)
	}
}

func quitCompanion(ctx context.Context, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	command := exec.CommandContext(ctx, path, companionQuitFlag)
	// A session of its own, so a Ctrl-C in the terminal does not end it.
	command.SysProcAttr = companionSysProcAttr()
	out, err := command.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == companionNotRunningExit:
		return false, nil
	default:
		return false, withWhatItSaid(err, out)
	}
}

// withWhatItSaid adds what a failed program printed to its error.
func withWhatItSaid(err error, out []byte) error {
	if said := strings.TrimSpace(string(out)); said != "" {
		return fmt.Errorf("%w: %s", err, said)
	}
	return err
}

// askCompanion starts the companion with one of the flags it takes as
// its only argument and waits for it to end.
func askCompanion(ctx context.Context, path, flag string) error {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	command := exec.CommandContext(ctx, path, flag)
	command.SysProcAttr = companionSysProcAttr()
	out, err := command.CombinedOutput()
	if err != nil {
		return withWhatItSaid(err, out)
	}
	return nil
}
