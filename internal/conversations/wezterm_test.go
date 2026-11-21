package conversations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// weztermList is `wezterm cli list --format json` with two windows, the
// second holding a tab split in two panes.
const weztermList = `[
  {"window_id": 0, "tab_id": 0, "pane_id": 0, "workspace": "default", "size": {"rows": 24, "cols": 80, "pixel_width": 640, "pixel_height": 384, "dpi": 72}, "title": "zsh", "cwd": "file:///Users/you", "cursor_x": 0, "cursor_y": 0, "cursor_shape": "Default", "cursor_visibility": "Visible", "left_col": 0, "top_row": 0, "tab_title": "", "window_title": "zsh", "is_active": true, "is_zoomed": false, "tty_name": "/dev/ttys040"},
  {"window_id": 1, "tab_id": 3, "pane_id": 5, "workspace": "default", "title": "zsh", "tab_title": "", "window_title": "zsh", "is_active": true, "is_zoomed": false, "tty_name": "/dev/ttys043"},
  {"window_id": 1, "tab_id": 3, "pane_id": 6, "workspace": "default", "title": "claude", "tab_title": "", "window_title": "zsh", "is_active": false, "is_zoomed": false, "tty_name": "/dev/ttys041"},
  {"window_id": 1, "tab_id": 4, "pane_id": 7, "workspace": "default", "title": "ssh", "tab_title": "", "window_title": "zsh", "is_active": false, "is_zoomed": false}
]`

type fakeCommands struct {
	calls []string
	out   map[string]string
	errs  map[string]error
}

func (f *fakeCommands) run(_ context.Context, name string, args ...string) (string, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, call)
	for prefix, err := range f.errs {
		if strings.HasPrefix(call, prefix) {
			return "", err
		}
	}
	for prefix, out := range f.out {
		if strings.HasPrefix(call, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func testWezTerm(commands *fakeCommands) weztermTab {
	return weztermTab{run: commands.run, path: func() (string, error) { return "wezterm", nil }}
}

func TestWezTerm_PicksThePaneByItsTTY(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList}}
	wezterm := testWezTerm(commands)

	ok, err := wezterm.focus(context.Background(), "/dev/ttys041")

	want := "wezterm cli --no-auto-start list --format json; wezterm cli --no-auto-start activate-pane --pane-id 6; open -a WezTerm"
	if !ok || err != nil || strings.Join(commands.calls, "; ") != want {
		t.Errorf("ok %v err %v calls %q", ok, err, commands.calls)
	}
}

func TestWezTerm_NoPaneOnTheTTY(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList}}
	wezterm := testWezTerm(commands)

	ok, err := wezterm.focus(context.Background(), "/dev/ttys099")

	if ok || err != nil || len(commands.calls) != 1 {
		t.Errorf("ok %v err %v calls %q", ok, err, commands.calls)
	}
}

func TestWezTerm_WithoutItsCommand(t *testing.T) {
	commands := &fakeCommands{}
	wezterm := weztermTab{run: commands.run, path: func() (string, error) { return "", errNoTabCommand }}

	_, err := wezterm.focus(context.Background(), "/dev/ttys041")

	if !errors.Is(err, errNoTabCommand) || len(commands.calls) != 0 {
		t.Errorf("err %v calls %q", err, commands.calls)
	}
}

func TestWezTerm_ACommandThatCannotReachTheAppIsNoTabCommand(t *testing.T) {
	for name, commands := range map[string]*fakeCommands{
		"list fails":      {errs: map[string]error{"wezterm cli --no-auto-start list": errors.New("failed to connect")}},
		"list unreadable": {out: map[string]string{"wezterm cli --no-auto-start list": "not json"}},
	} {
		if _, err := testWezTerm(commands).focus(context.Background(), "/dev/ttys041"); !errors.Is(err, errNoTabCommand) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	quit := &fakeCommands{errs: map[string]error{"wezterm cli --no-auto-start spawn": errors.New("failed to connect")}}
	if err := testWezTerm(quit).newTab(context.Background(), "claude --resume x"); !errors.Is(err, errNoTabCommand) {
		t.Errorf("spawn: err = %v", err)
	}
}

func TestWezTerm_TypesOnlyOnceThePaneIsInFront(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList}, errs: map[string]error{"open -a WezTerm": errors.New("no such app")}}

	ok, err := testWezTerm(commands).typeIn(context.Background(), "/dev/ttys041", "hello")

	if ok || err == nil {
		t.Errorf("ok %v err %v", ok, err)
	}
	for _, call := range commands.calls {
		if strings.Contains(call, "send-text") {
			t.Errorf("typed although WezTerm did not come forward: %q", commands.calls)
		}
	}
}

func TestWezTerm_TitlesTypesAndOpensWithEveryValueAsItsOwnArgument(t *testing.T) {
	hostile := "--pane-id 0 \"x\"; rm -rf ~\nsecond line"
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList, "wezterm cli --no-auto-start spawn": "8\n"}}
	wezterm := testWezTerm(commands)

	if ok, err := wezterm.title(context.Background(), "/dev/ttys041", hostile); !ok || err != nil {
		t.Fatalf("title: ok %v err %v", ok, err)
	}
	if ok, err := wezterm.typeIn(context.Background(), "/dev/ttys041", hostile); !ok || err != nil {
		t.Fatalf("type: ok %v err %v", ok, err)
	}
	if err := wezterm.newTab(context.Background(), hostile); err != nil {
		t.Fatalf("new tab: %v", err)
	}

	want := []string{
		"wezterm cli --no-auto-start list --format json",
		"wezterm cli --no-auto-start set-tab-title --pane-id 6 -- " + hostile,
		"wezterm cli --no-auto-start list --format json",
		"wezterm cli --no-auto-start activate-pane --pane-id 6",
		"open -a WezTerm",
		"wezterm cli --no-auto-start send-text --pane-id 6 --no-paste -- \x1b[200~" + hostile + "\x1b[201~",
		"wezterm cli --no-auto-start spawn",
		"wezterm cli --no-auto-start send-text --pane-id 8 --no-paste -- " + hostile + "\r",
		"open -a WezTerm",
	}
	if strings.Join(commands.calls, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("calls:\n%q\nwant:\n%q", commands.calls, want)
	}
}

func TestWezTermArgs_KeepTheValueLastAfterTheEndOfOptions(t *testing.T) {
	hostile := "--no-paste"
	for name, args := range map[string][]string{
		"title": weztermTitleArgs(6, hostile),
		"type":  weztermSendArgs(6, hostile),
	} {
		if len(args) < 2 || args[len(args)-1] != hostile || args[len(args)-2] != "--" {
			t.Errorf("%s: %q", name, args)
		}
	}
}

// linuxArgMax is the largest single argument Linux passes to a program.
const linuxArgMax = 128 << 10

// sentParts are the values after "-- " of every send-text call, in order.
func sentParts(calls []string) []string {
	var parts []string
	for _, call := range calls {
		if strings.Contains(call, " send-text ") {
			_, value, _ := strings.Cut(call, " -- ")
			parts = append(parts, value)
		}
	}
	return parts
}

func TestWezTerm_TypesALargeTextInPartsThatFitOneArgument(t *testing.T) {
	text := strings.Repeat("aé\\", MaxSendBytes/4)
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList}}

	if ok, err := testWezTerm(commands).typeIn(context.Background(), "/dev/ttys041", text); !ok || err != nil {
		t.Fatalf("ok %v err %v", ok, err)
	}

	parts := sentParts(commands.calls)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want the text cut", len(parts))
	}
	for _, part := range parts {
		if len(part) >= linuxArgMax || !utf8.ValidString(part) {
			t.Errorf("part of %d bytes, valid UTF-8 %v", len(part), utf8.ValidString(part))
		}
	}
	if got := strings.Join(parts, ""); got != "\x1b[200~"+text+"\x1b[201~" {
		t.Errorf("joined parts differ from the pasted text")
	}
}
