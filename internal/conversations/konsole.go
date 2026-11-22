package conversations

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Konsole puts these in the environment of every shell it starts: the
// D-Bus name of its process and the path of the tab's session object.
const (
	konsoleServiceVar = "KONSOLE_DBUS_SERVICE"
	konsoleSessionVar = "KONSOLE_DBUS_SESSION"
)

// errKonsoleCommandsOff is Konsole refusing text and commands sent over
// D-Bus, which it does until the person turns the setting on. It does
// not count as errNoTabCommand: the tab can still be selected and named.
var errKonsoleCommandsOff = errors.New("Konsole refuses text and commands sent to it. Turn on Enable the security sensitive parts of the DBus API in Konsole's settings, under General") //nolint:staticcheck // the text is shown as a sentence

var (
	konsoleServicePattern = regexp.MustCompile(`^(:[0-9]+\.[0-9]+|org\.kde\.konsole(-[0-9]+)?)$`)
	konsoleSessionPattern = regexp.MustCompile(`^/Sessions/([0-9]+)$`)
	dbusNodePattern       = regexp.MustCompile(`<node name="([0-9]+)"/>`)
	dbusStringPattern     = regexp.MustCompile(`(?m)^\s*string "([0-9]+)"$`)
	dbusInt32Pattern      = regexp.MustCompile(`(?m)^\s*int32 (-?[0-9]+)$`)
)

// konsoleTab drives Konsole over the D-Bus session bus with dbus-send,
// which ships with D-Bus itself. Konsole calls a tab a session.
type konsoleTab struct {
	run     func(ctx context.Context, name string, args ...string) (string, error)
	table   func() ([]ProcessInfo, error)
	environ func(pids []int) (map[int][]string, error)
}

func (k konsoleTab) call(ctx context.Context, service, path, method string, args ...string) (string, error) {
	out, err := k.run(ctx, "dbus-send", append([]string{"--session", "--print-reply", "--dest=" + service, path, method}, args...)...)
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "", fmt.Errorf("%w: %w", errNoTabCommand, err)
	// Konsole translates its reason, so its error name tells the refusal.
	case err != nil && strings.Contains(err.Error(), "org.freedesktop.DBus.Error.AccessDenied"):
		return "", errKonsoleCommandsOff
	}
	return out, err
}

// address is the Konsole process and session that run a process on tty,
// or of any Konsole tab when tty is empty, from their environment.
func (k konsoleTab) address(tty string) (service, session string, err error) {
	table, err := k.table()
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	var pids []int
	for _, process := range table {
		if tty != "" && process.TTY == tty || tty == "" && programOf(byPID[process.PPID].Command) == "konsole" {
			pids = append(pids, process.PID)
		}
	}
	slices.Sort(pids)
	for _, pid := range pids {
		// A process of another user, such as one run with sudo, keeps
		// its environment from us; it is skipped.
		environments, err := k.environ([]int{pid})
		if err != nil {
			continue
		}
		values := map[string]string{}
		for _, entry := range environments[pid] {
			if name, value, ok := strings.Cut(entry, "="); ok {
				values[name] = value
			}
		}
		service, session := values[konsoleServiceVar], values[konsoleSessionVar]
		if konsoleServicePattern.MatchString(service) && konsoleSessionPattern.MatchString(session) {
			return service, konsoleSessionPattern.FindStringSubmatch(session)[1], nil
		}
	}
	// A Konsole built without D-Bus, or run from a Flatpak, sets neither.
	return "", "", fmt.Errorf("%w: no Konsole session in the environment", errNoTabCommand)
}

func (k konsoleTab) windows(ctx context.Context, service string) ([]string, error) {
	out, err := k.call(ctx, service, "/Windows", "org.freedesktop.DBus.Introspectable.Introspect")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	var windows []string
	for _, match := range dbusNodePattern.FindAllStringSubmatch(out, -1) {
		windows = append(windows, "/Windows/"+match[1])
	}
	return windows, nil
}

// window is the window that holds session; ok is false when none does.
func (k konsoleTab) window(ctx context.Context, service, session string) (window string, ok bool, err error) {
	windows, err := k.windows(ctx, service)
	if err != nil {
		return "", false, err
	}
	for _, window := range windows {
		out, err := k.call(ctx, service, window, "org.kde.konsole.Window.sessionList")
		if err != nil {
			return "", false, err
		}
		for _, match := range dbusStringPattern.FindAllStringSubmatch(out, -1) {
			if match[1] == session {
				return window, true, nil
			}
		}
	}
	return "", false, nil
}

func (k konsoleTab) selectTab(ctx context.Context, tty string) (service, session string, ok bool, err error) {
	service, session, err = k.address(tty)
	if err != nil {
		return "", "", false, err
	}
	window, ok, err := k.window(ctx, service, session)
	if err != nil || !ok {
		return "", "", false, err
	}
	_, err = k.call(ctx, service, window, "org.kde.konsole.Window.setCurrentSession", "int32:"+session)
	return service, session, err == nil, err
}

// focus selects the tab. agnostic-ai does not bring Konsole's window
// forward, so the reply says it may be behind.
func (k konsoleTab) focus(ctx context.Context, tty string) (bool, error) {
	_, _, ok, err := k.selectTab(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	return true, errNotInFront
}

// title sets the tab title, which Konsole calls role 1.
func (k konsoleTab) title(ctx context.Context, tty, title string) (bool, error) {
	service, session, err := k.address(tty)
	if err != nil {
		return false, err
	}
	_, err = k.call(ctx, service, "/Sessions/"+session, "org.kde.konsole.Session.setTitle", "int32:1", "string:"+konsoleTitle(title))
	return err == nil, err
}

// konsoleTitle keeps a percent sign as it reads: Konsole keeps the title
// as a format, where %d or %n stand for the folder or program, and has no
// escape for one, so a full-width percent sign takes its place.
func konsoleTitle(title string) string { return strings.ReplaceAll(title, "%", "\uff05") }

// typeIn selects the tab and sends text as a bracketed paste, which
// Konsole passes on as it is, so a line break does not submit it.
func (k konsoleTab) typeIn(ctx context.Context, tty, text string) (bool, error) {
	service, session, ok, err := k.selectTab(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	for i, part := range textParts("\x1b[200~"+text+"\x1b[201~", sendPart) {
		if _, err := k.call(ctx, service, "/Sessions/"+session, "org.kde.konsole.Session.sendText", "string:"+part); err != nil {
			if i > 0 {
				closePaste(ctx, func(ctx context.Context) error {
					_, err := k.call(ctx, service, "/Sessions/"+session, "org.kde.konsole.Session.sendText", "string:\x1b[201~")
					return err
				})
			}
			return false, err
		}
	}
	return true, nil
}

// closePaste ends a paste a failed part left open, which would swallow
// the next keys typed in the tab, even after the send timed out.
func closePaste(ctx context.Context, send func(context.Context) error) {
	closing, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = send(closing)
}

// sendPart bounds each piece of sent text: Linux refuses a single
// argument over 128 KB, and a send may be larger.
const sendPart = 64 << 10

// textParts cuts text into pieces of at most size bytes, never inside a
// character.
func textParts(text string, size int) []string {
	var parts []string
	for len(text) > size {
		cut := size
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		parts = append(parts, text[:cut])
		text = text[cut:]
	}
	return append(parts, text)
}

// newTab opens a tab in the first Konsole window, in the folder the
// line changes to, and runs the rest of the line there.
func (k konsoleTab) newTab(ctx context.Context, command string) error {
	service, _, err := k.address("")
	if err != nil {
		return err
	}
	windows, err := k.windows(ctx, service)
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		return fmt.Errorf("%w: Konsole has no window", errNoTabCommand)
	}
	folder, rest := "", command
	if after, ok := strings.CutPrefix(command, "cd "); ok {
		if quoted, tail, ok := cutShellQuoted(after); ok {
			if line, ok := strings.CutPrefix(tail, " && "); ok {
				folder, rest = quoted, line
			}
		}
	}
	out, err := k.call(ctx, service, windows[0], "org.kde.konsole.Window.newSession", "string:", "string:"+folder)
	if err != nil {
		return fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	match := dbusInt32Pattern.FindStringSubmatch(out)
	if match == nil {
		return fmt.Errorf("%w: Konsole named no new session", errNoTabCommand)
	}
	if _, err := strconv.Atoi(match[1]); err != nil {
		return err
	}
	_, err = k.call(ctx, service, "/Sessions/"+match[1], "org.kde.konsole.Session.runCommand", "string:"+rest)
	return err
}
