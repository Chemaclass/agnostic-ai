package conversations

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// errNoTabCommand means the command that drives a tab app is missing
// or could not reach the app, as when the app is not running.
var errNoTabCommand = errors.New("its command could not reach the app")

// tabSetupError says what the person must change before a tab app
// can be driven. It counts as errNoTabCommand.
type tabSetupError string

func (e tabSetupError) Error() string { return string(e) }

func (e tabSetupError) Is(target error) bool { return target == errNoTabCommand }

// commandTab drives a tab app through its own command instead of
// AppleScript. Each value goes in as an argument of its own.
type commandTab interface {
	focus(ctx context.Context, tty string) (bool, error)
	title(ctx context.Context, tty, title string) (bool, error)
	typeIn(ctx context.Context, tty, text string) (bool, error)
	newTab(ctx context.Context, command string) error
}

type tabNameReader interface {
	readTabNames(context.Context) ([]TerminalTab, error)
}

func macCommandTabNameReader(app string) (tabNameReader, bool) {
	driver, ok := commandTabs[app].(tabNameReader)
	return driver, ok
}

var commandTabs = map[string]commandTab{
	"WezTerm": weztermTab{run: run, runInput: runInput, path: func() (string, error) { return bundledCommand("WezTerm.app", "wezterm") }},
	"kitty":   kittyTab{run: run, path: func() (string, error) { return bundledCommand("kitty.app", "kitty") }, table: SystemProcesses{}.Table, socket: ownSocket},
}

// linuxCommandTabs drive the same apps on Linux, from PATH, without
// bringing their windows forward.
var linuxCommandTabs = map[string]commandTab{
	"WezTerm": weztermTab{run: run, runInput: runInput, path: func() (string, error) { return commandOnPath("wezterm") }, noRaising: true},
	"kitty":   kittyTab{run: run, path: func() (string, error) { return commandOnPath("kitty") }, table: SystemProcesses{}.Table, socket: ownSocket, noRaising: true},
	"Konsole": konsoleTab{run: run, table: SystemProcesses{}.Table, environ: readEnvironments},
}

// readEnvironments reads the kept entries of each pid's environment, on
// a system with a reader.
func readEnvironments(pids []int) (map[int][]string, error) {
	if environmentReader == nil {
		return nil, errors.New(string(environmentUnsupported))
	}
	return environmentReader(pids)
}

func commandOnPath(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", errNoTabCommand
}

// bundledCommand finds an app's command. The companion started from
// Finder has no shell PATH, so the app bundles come first.
func bundledCommand(bundle, name string) (string, error) {
	candidates := []string{filepath.Join("/Applications", bundle, "Contents", "MacOS", name)}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Applications", bundle, "Contents", "MacOS", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", errNoTabCommand
}
