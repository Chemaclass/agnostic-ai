package conversations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// tmuxServers finds a pane among the tmux servers of this user. A server
// is reached by its socket, and the server's pid tells which socket.
type tmuxServers struct {
	run     func(ctx context.Context, name string, args ...string) (string, error)
	path    func() (string, error)
	sockets func() []string
}

// tmuxPath finds tmux. The companion started from Finder has no shell
// PATH, so Homebrew's folders come first.
func tmuxPath() (string, error) {
	for _, candidate := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("tmux"); err == nil {
		return path, nil
	}
	return "", errNoTabCommand
}

// tmuxSockets lists the sockets in the folder where tmux keeps them for
// this user, as tmux names it from TMUX_TMPDIR or /tmp.
func tmuxSockets() []string {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	dir = filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var sockets []string
	for _, entry := range entries {
		if entry.Type()&os.ModeSocket != 0 {
			sockets = append(sockets, filepath.Join(dir, entry.Name()))
		}
	}
	return sockets
}

func (t tmuxServers) tmux(ctx context.Context, socket string, args ...string) (string, error) {
	path, err := t.path()
	if err != nil {
		return "", err
	}
	return t.run(ctx, path, append([]string{"-S", socket}, args...)...)
}

// show selects the pane on tty of the server with pid server, in the
// session of the most recently active client attached to it, and returns
// that client's pid, or 0 when the pane is in no attached session.
func (t tmuxServers) show(ctx context.Context, server int, tty string) (int, error) {
	if _, err := t.path(); err != nil {
		return 0, err
	}
	for _, socket := range t.sockets() {
		pane, windows := t.pane(ctx, socket, server, tty)
		if pane == "" {
			continue
		}
		client, session, err := t.client(ctx, socket, windows)
		if err != nil || client == 0 {
			return 0, err
		}
		if _, err := t.tmux(ctx, socket, "select-window", "-t", session+":"+windows[session]); err != nil {
			return 0, err
		}
		if _, err := t.tmux(ctx, socket, "select-pane", "-t", pane); err != nil {
			return 0, err
		}
		return client, nil
	}
	return 0, nil
}

// pane finds the pane on tty when socket belongs to server, with its
// window in each session that lists it: grouped sessions share windows.
// A socket with no server behind it answers with an error and is skipped.
func (t tmuxServers) pane(ctx context.Context, socket string, server int, tty string) (pane string, windows map[string]string) {
	out, err := t.tmux(ctx, socket, "list-panes", "-a", "-F", "#{pid}\t#{pane_tty}\t#{pane_id}\t#{session_id}\t#{window_id}")
	if err != nil {
		return "", nil
	}
	windows = map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 5 && fields[0] == strconv.Itoa(server) && fields[1] == tty {
			pane, windows[fields[3]] = fields[2], fields[4]
		}
	}
	return pane, windows
}

// client picks the most recently active client attached to one of the
// sessions in windows, and that session.
func (t tmuxServers) client(ctx context.Context, socket string, windows map[string]string) (client int, session string, err error) {
	out, err := t.tmux(ctx, socket, "list-clients", "-F", "#{client_pid}\t#{session_id}\t#{client_activity}")
	if err != nil {
		return 0, "", fmt.Errorf("tmux clients: %w", err)
	}
	latest := int64(-1)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || windows[fields[1]] == "" {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		activity, activityErr := strconv.ParseInt(fields[2], 10, 64)
		if errors.Join(pidErr, activityErr) == nil && activity > latest {
			client, session, latest = pid, fields[1], activity
		}
	}
	return client, session, nil
}
