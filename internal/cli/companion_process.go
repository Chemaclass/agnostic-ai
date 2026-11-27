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
	if bundle, inside := companionAppForProgram(path); goos == "darwin" && inside && knownCompanionExecutable(filepath.Base(path)) {
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

// companionProgramRuns reports whether this user's process list holds the
// program at path.
func companionProgramRuns(ctx context.Context, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-xo", "comm=").Output()
	if err != nil {
		return false, fmt.Errorf("list the running programs: %w", err)
	}
	return programListed(string(out), path), nil
}

// Match app executables by their resolved paths so another copy cannot
// make an update look successful. Bare programs still match by name.
func programListed(list, path string) bool {
	name := filepath.Base(path)
	_, insideApp := companionAppForProgram(path)
	if insideApp {
		path = resolvedPath(path)
	}
	for line := range strings.Lines(list) {
		line = strings.TrimSpace(line)
		if filepath.Base(line) != name {
			continue
		}
		if !insideApp || filepath.IsAbs(line) && resolvedPath(line) == path {
			return true
		}
	}
	return false
}

// companionAppForProgram returns the app that holds the program at path.
func companionAppForProgram(path string) (string, bool) {
	macOS := filepath.Dir(path)
	contents := filepath.Dir(macOS)
	app := filepath.Dir(contents)
	return app, filepath.Base(macOS) == "MacOS" && filepath.Base(contents) == "Contents" && filepath.Ext(app) == ".app"
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

func companionLoginStatus(ctx context.Context, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	// An earlier build still answers --version without opening the app.
	command := exec.CommandContext(ctx, path, "--version", "--start-at-login-status")
	command.SysProcAttr = companionSysProcAttr()
	out, err := command.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			err = withWhatItSaid(err, exit.Stderr)
		}
		return false, fmt.Errorf("ask %s whether start at login is on: %w", path, err)
	}
	switch strings.TrimSpace(string(out)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s did not report whether start at login is on", path)
	}
}
