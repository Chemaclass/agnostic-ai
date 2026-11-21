package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// errKittyRemoteControlOff means kitty listens on no socket the server
// knows, which is how kitty runs until remote control is turned on.
var errKittyRemoteControlOff error = tabSetupError("remote control is off. Add allow_remote_control socket-only and listen_on unix:/tmp/kitty-{kitty_pid} to kitty.conf, then restart kitty")

// kittyTab drives kitty with `kitty @` over the socket that
// listen_on unix:/tmp/kitty-{kitty_pid} or unix:/tmp/kitty in kitty.conf
// opens. kitty calls a pane a window.
type kittyTab struct {
	run    func(ctx context.Context, name string, args ...string) (string, error)
	path   func() (string, error)
	table  func() ([]ProcessInfo, error)
	socket func(path string) bool
	// noRaising is set on Linux, where agnostic-ai does not bring kitty's
	// window forward: Wayland lets no program raise another's window.
	noRaising bool
}

// raise brings kitty's window forward, or says it may stay behind.
func (k kittyTab) raise(ctx context.Context) error {
	if k.noRaising {
		return errNotInFront
	}
	_, err := k.run(ctx, "open", "-a", "kitty")
	return err
}

// kittyText keeps backslashes, which kitty reads as escapes in the
// values it takes as arguments.
func kittyText(text string) string { return strings.ReplaceAll(text, `\`, `\\`) }

func isKitty(command string) bool {
	return strings.HasSuffix(command, "/kitty.app/Contents/MacOS/kitty") || programOf(command) == "kitty"
}

// remote is the address of the kitty that runs tty, or when tty is
// empty of the first kitty that listens: on Linux each kitty started
// alone is a process of its own. pids holds the processes on tty.
func (k kittyTab) remote(tty string) (to string, pids map[int]bool, err error) {
	table, err := k.table()
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	pids = map[int]bool{}
	kitty := 0
	for _, process := range table {
		switch {
		case tty == "" && isKitty(process.Command) && !isKitty(byPID[process.PPID].Command):
			if kitty == 0 || !k.socket(kittySocket(kitty)) {
				kitty = process.PID
			}
		case tty != "" && process.TTY == tty:
			pids[process.PID] = true
			if kitty == 0 && isKitty(byPID[process.PPID].Command) {
				kitty = process.PPID
			}
		}
	}
	if kitty == 0 {
		return "", pids, nil
	}
	socket := kittySocket(kitty)
	if !k.socket(socket) {
		return "", pids, errKittyRemoteControlOff
	}
	return "unix:" + socket, pids, nil
}

func kittySocket(pid int) string { return "/tmp/kitty-" + strconv.Itoa(pid) }

func (k kittyTab) at(ctx context.Context, to string, args ...string) (string, error) {
	path, err := k.path()
	if err != nil {
		return "", err
	}
	return k.run(ctx, path, append([]string{"@", "--to", to}, args...)...)
}

// reach runs the first command of a call, whose failure means kitty
// cannot be driven at all.
func (k kittyTab) reach(ctx context.Context, to string, args ...string) (string, error) {
	out, err := k.at(ctx, to, args...)
	if err != nil && !errors.Is(err, errNoTabCommand) {
		err = fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	return out, err
}

// window finds the kitty window whose first process runs on tty; ok is
// false when none does.
func (k kittyTab) window(ctx context.Context, tty string) (to, window string, ok bool, err error) {
	to, pids, err := k.remote(tty)
	if err != nil || to == "" {
		return "", "", false, err
	}
	out, err := k.reach(ctx, to, "ls")
	if err != nil {
		return "", "", false, err
	}
	var osWindows []struct {
		Tabs []struct {
			Windows []struct {
				ID  int `json:"id"`
				PID int `json:"pid"`
			} `json:"windows"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal([]byte(out), &osWindows); err != nil {
		return "", "", false, fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	for _, osWindow := range osWindows {
		for _, tab := range osWindow.Tabs {
			for _, listed := range tab.Windows {
				if pids[listed.PID] {
					return to, strconv.Itoa(listed.ID), true, nil
				}
			}
		}
	}
	return "", "", false, nil
}

func (k kittyTab) activate(ctx context.Context, to, window string) error {
	if _, err := k.at(ctx, to, "focus-window", "--match", "id:"+window); err != nil {
		return err
	}
	return k.raise(ctx)
}

func (k kittyTab) focus(ctx context.Context, tty string) (bool, error) {
	to, window, ok, err := k.window(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	return true, k.activate(ctx, to, window)
}

func (k kittyTab) title(ctx context.Context, tty, title string) (bool, error) {
	to, window, ok, err := k.window(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	_, err = k.at(ctx, to, "set-tab-title", "--match", "window_id:"+window, "--", kittyText(title))
	return err == nil, err
}

// typeIn pastes text once the window is in front. kitten sends text in
// parts of 2048 bytes, and kitty's own bracketed paste would wrap each
// part, so the markers go around the whole text here; every tool typed
// into asks for bracketed paste.
func (k kittyTab) typeIn(ctx context.Context, tty, text string) (bool, error) {
	to, window, ok, err := k.window(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	if err := k.activate(ctx, to, window); err != nil && !errors.Is(err, errNotInFront) {
		return false, err
	}
	_, err = k.at(ctx, to, "send-text", "--match", "id:"+window, "--", `\e[200~`+kittyText(text)+`\e[201~`)
	return err == nil, err
}

// newTab opens a tab in kitty's active window and types command and
// Enter into its shell.
func (k kittyTab) newTab(ctx context.Context, command string) error {
	to, _, err := k.remote("")
	if err != nil {
		return err
	}
	if to == "" {
		return errNoTabCommand
	}
	out, err := k.reach(ctx, to, "launch", "--type", "tab")
	if err != nil {
		return err
	}
	window := strings.TrimSpace(out)
	if _, err := strconv.Atoi(window); err != nil {
		return err
	}
	if _, err := k.at(ctx, to, "send-text", "--match", "id:"+window, "--", kittyText(command)+`\r`); err != nil {
		return err
	}
	if err := k.raise(ctx); !errors.Is(err, errNotInFront) {
		return err
	}
	return nil
}
