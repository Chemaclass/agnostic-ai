package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// weztermTab drives WezTerm with `wezterm cli`. --no-auto-start keeps
// it from starting a hidden mux server when the app is not running.
type weztermTab struct {
	run  func(ctx context.Context, name string, args ...string) (string, error)
	path func() (string, error)
	// noRaising is set where WezTerm's window cannot be brought forward
	// from outside, as on Linux, where only macOS's open -a can.
	noRaising bool
}

// raise brings WezTerm's window forward, or says it may stay behind.
func (w weztermTab) raise(ctx context.Context) error {
	if w.noRaising {
		return errNotInFront
	}
	_, err := w.run(ctx, "open", "-a", "WezTerm")
	return err
}

func weztermCLI(args ...string) []string {
	return append([]string{"cli", "--no-auto-start"}, args...)
}

func weztermTitleArgs(pane uint64, title string) []string {
	return weztermCLI("set-tab-title", "--pane-id", strconv.FormatUint(pane, 10), "--", title)
}

// weztermSendArgs send text as a paste, which WezTerm brackets when the
// program asks for it, or as typed keys.
func weztermSendArgs(pane uint64, text string, paste bool) []string {
	args := []string{"send-text", "--pane-id", strconv.FormatUint(pane, 10)}
	if !paste {
		args = append(args, "--no-paste")
	}
	return weztermCLI(append(args, "--", text)...)
}

func (w weztermTab) cli(ctx context.Context, args ...string) (string, error) {
	path, err := w.path()
	if err != nil {
		return "", err
	}
	return w.run(ctx, path, args...)
}

// reach runs the first command of a call, whose failure means the app
// cannot be driven at all.
func (w weztermTab) reach(ctx context.Context, args ...string) (string, error) {
	out, err := w.cli(ctx, args...)
	if err != nil && !errors.Is(err, errNoTabCommand) {
		err = fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	return out, err
}

// pane finds the pane whose terminal is tty; ok is false when none is.
func (w weztermTab) pane(ctx context.Context, tty string) (pane uint64, ok bool, err error) {
	out, err := w.reach(ctx, weztermCLI("list", "--format", "json")...)
	if err != nil {
		return 0, false, err
	}
	var panes []struct {
		PaneID  uint64 `json:"pane_id"`
		TTYName string `json:"tty_name"`
	}
	if err := json.Unmarshal([]byte(out), &panes); err != nil {
		return 0, false, fmt.Errorf("%w: %w", errNoTabCommand, err)
	}
	for _, listed := range panes {
		if listed.TTYName != "" && shortTTY(listed.TTYName) == shortTTY(tty) {
			return listed.PaneID, true, nil
		}
	}
	return 0, false, nil
}

func (w weztermTab) activate(ctx context.Context, pane uint64) error {
	if _, err := w.cli(ctx, weztermCLI("activate-pane", "--pane-id", strconv.FormatUint(pane, 10))...); err != nil {
		return err
	}
	return w.raise(ctx)
}

func (w weztermTab) focus(ctx context.Context, tty string) (bool, error) {
	pane, ok, err := w.pane(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	return true, w.activate(ctx, pane)
}

func (w weztermTab) title(ctx context.Context, tty, title string) (bool, error) {
	pane, ok, err := w.pane(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	_, err = w.cli(ctx, weztermTitleArgs(pane, title)...)
	return err == nil, err
}

func (w weztermTab) typeIn(ctx context.Context, tty, text string) (bool, error) {
	pane, ok, err := w.pane(ctx, tty)
	if err != nil || !ok {
		return false, err
	}
	if err := w.activate(ctx, pane); err != nil && !errors.Is(err, errNotInFront) {
		return false, err
	}
	_, err = w.cli(ctx, weztermSendArgs(pane, text, true)...)
	return err == nil, err
}

// newTab spawns a shell in a new tab of the window of WEZTERM_PANE, set
// when the server runs inside WezTerm, else of the pane last in focus,
// and types command and Enter into it.
func (w weztermTab) newTab(ctx context.Context, command string) error {
	out, err := w.reach(ctx, weztermCLI("spawn")...)
	if err != nil {
		return err
	}
	pane, err := strconv.ParseUint(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return err
	}
	if _, err := w.cli(ctx, weztermSendArgs(pane, command+"\r", false)...); err != nil {
		return err
	}
	if err := w.raise(ctx); !errors.Is(err, errNotInFront) {
		return err
	}
	return nil
}
