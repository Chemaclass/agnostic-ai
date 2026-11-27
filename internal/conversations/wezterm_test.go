package conversations

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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
	// inputs holds what each call read on standard input, by call index.
	inputs map[int]string
	out    map[string]string
	errs   map[string]error
}

func (f *fakeCommands) runInput(ctx context.Context, input, name string, args ...string) (string, error) {
	if f.inputs == nil {
		f.inputs = map[int]string{}
	}
	f.inputs[len(f.calls)] = input
	return f.run(ctx, name, args...)
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
	return weztermTab{run: commands.run, runInput: commands.runInput, path: func() (string, error) { return "wezterm", nil }}
}

func useWezTermList(t *testing.T, output string) *fakeCommands {
	t.Helper()
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": output}}
	driver := commandTabs["WezTerm"]
	t.Cleanup(func() { commandTabs["WezTerm"] = driver })
	commandTabs["WezTerm"] = testWezTerm(commands)
	return commands
}

func TestMacLauncher_ReadsExplicitWezTermTitlesInOneList(t *testing.T) {
	commands := useWezTermList(t, `[
	  {"tab_id":0,"pane_id":0,"title":"shell title","tab_title":"Typed tab name","tty_name":"/dev/ttys004"},
	  {"tab_id":2,"pane_id":4,"title":"another pane title","tab_title":"","tty_name":"/dev/ttys005"},
	  {"tab_id":1,"pane_id":1,"title":"shell","tab_title":"Split","tty_name":"/dev/ttys006"},
	  {"tab_id":1,"pane_id":2,"title":"ssh","tab_title":"Split","tty_name":null}
	]`)

	tabs, err := (macLauncher{}).ReadTabNames(context.Background(), "WezTerm")

	want := []TerminalTab{
		tab("Typed tab name", "/dev/ttys004"),
		tab("", "/dev/ttys005"),
		tab("Split", "/dev/ttys006", ""),
	}
	if err != nil || !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs = %+v, err = %v, want %+v", tabs, err, want)
	}
	if want := []string{"wezterm cli --no-auto-start list --format json"}; !reflect.DeepEqual(commands.calls, want) {
		t.Errorf("calls = %q, want %q", commands.calls, want)
	}
}

func TestMacLauncher_RejectsAmbiguousWezTermSnapshots(t *testing.T) {
	for name, output := range map[string]string{
		"no tab ID":         `[{"tab_title":"Typed","tty_name":"/dev/ttys004"}]`,
		"no explicit title": `[{"tab_id":0,"title":"shell","tty_name":"/dev/ttys004"}]`,
		"conflicting titles": `[
		  {"tab_id":0,"tab_title":"First","tty_name":"/dev/ttys004"},
		  {"tab_id":0,"tab_title":"Second","tty_name":"/dev/ttys005"}
		]`,
	} {
		t.Run(name, func(t *testing.T) {
			useWezTermList(t, output)
			tabs, err := (macLauncher{}).ReadTabNames(context.Background(), "WezTerm")
			if len(tabs) != 0 || !errors.Is(err, errNoTabCommand) {
				t.Errorf("tabs = %+v, err = %v, want an unusable snapshot", tabs, err)
			}
		})
	}
}

func TestSyncTabNames_WezTermFollowsItsTabWithF2ProtectionOnMacOS(t *testing.T) {
	const list = "wezterm cli --no-auto-start list"
	const twoTabs = `[
	  {"tab_id":0,"pane_id":0,"title":"shell","tab_title":"Seen before F2","tty_name":"/dev/ttys004"},
	  {"tab_id":1,"pane_id":1,"title":"other shell","tab_title":"","tty_name":"/dev/ttys005"}
	]`
	commands := useWezTermList(t, twoTabs)
	s := newTabSync(t)
	table := ghosttyTable(
		ghosttyTab("/dev/ttys004", 45469, 45864, "claude"),
		ghosttyTab("/dev/ttys005", 45569, 45964, "claude"),
	)
	table[0].Command = "/Applications/WezTerm.app/Contents/MacOS/wezterm-gui"
	s.scanner.Processes = processesWithTable(table)
	ctx := context.Background()
	s.scanner.Scan(ctx, ScanOptions{})
	failures := s.scanner.SyncTabNames(ctx, macLauncher{}, s.inTool)

	if runtime.GOOS != "darwin" {
		if len(commands.calls) != 0 || len(s.named) != 0 {
			t.Errorf("outside macOS, calls = %q, renamed = %q", commands.calls, s.named)
		}
		return
	}
	if want := []string{idSafari + " Seen before F2"}; len(failures) != 0 || !reflect.DeepEqual(s.named, want) {
		t.Fatalf("failures = %v, renamed = %q, want %q", failures, s.named, want)
	}
	if title := s.title(t, idCache); title != "Fix the build" {
		t.Errorf("the other chat's title = %q", title)
	}

	writer := pendingTabWrite{fakeLauncher: s.launcher, started: make(chan struct{}), finish: make(chan error)}
	defer close(writer.finish)
	if _, err := s.scanner.RenameChat(ctx, ChatRename{
		Tool: ToolClaude, ID: idSafari, Title: "from F2", InTool: s.inTool, TabLauncher: writer,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("the F2 tab write did not start")
	}
	commands.out[list] = strings.Replace(twoTabs, "Seen before F2", "Unread older title", 1)
	if failures := s.scanner.SyncTabNames(ctx, macLauncher{}, s.inTool); len(failures) != 0 {
		t.Errorf("reverse sync during F2: %v", failures)
	}
	if want := []string{idSafari + " Seen before F2", idSafari + " from F2"}; !reflect.DeepEqual(s.named, want) {
		t.Errorf("renamed = %q, want %q", s.named, want)
	}
	if title := s.title(t, idSafari); title != "from F2" {
		t.Errorf("chat title while the F2 tab write is pending = %q", title)
	}
}

func TestWezTerm_FocusesWithOnlyPaneData(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{
		"wezterm cli --no-auto-start list": `[{"pane_id":0,"tty_name":"/dev/ttys004"}]`,
	}}
	ok, err := testWezTerm(commands).focus(context.Background(), "/dev/ttys004")
	if !ok || err != nil {
		t.Errorf("focus with no tab metadata: ok = %v, err = %v", ok, err)
	}
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
		"wezterm cli --no-auto-start send-text --pane-id 6",
		"wezterm cli --no-auto-start spawn",
		"wezterm cli --no-auto-start send-text --pane-id 8 --no-paste -- " + hostile + "\r",
		"open -a WezTerm",
	}
	if strings.Join(commands.calls, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("calls:\n%q\nwant:\n%q", commands.calls, want)
	}
	if got := commands.inputs[5]; got != hostile {
		t.Errorf("pasted on standard input %q, want %q", got, hostile)
	}
}

func TestWezTermArgs_KeepTheValueLastAfterTheEndOfOptions(t *testing.T) {
	hostile := "--no-paste"
	for name, args := range map[string][]string{
		"title": weztermTitleArgs(6, hostile),
		"type":  weztermTypeArgs(6, hostile),
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

func TestWezTerm_PastesALargeTextInOneSendOnStandardInput(t *testing.T) {
	text := strings.Repeat("aé\\", MaxSendBytes/4)
	if len(text) <= linuxArgMax {
		t.Fatalf("text of %d bytes fits one argument", len(text))
	}
	commands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": weztermList}}

	if ok, err := testWezTerm(commands).typeIn(context.Background(), "/dev/ttys041", text); !ok || err != nil {
		t.Fatalf("ok %v err %v", ok, err)
	}

	var sends []int
	for i, call := range commands.calls {
		if strings.Contains(call, " send-text ") {
			sends = append(sends, i)
		}
	}
	if len(sends) != 1 {
		t.Fatalf("send-text calls = %d, want 1: %v", len(sends), sends)
	}
	if call := commands.calls[sends[0]]; call != "wezterm cli --no-auto-start send-text --pane-id 6" {
		t.Errorf("call %.200q carries the text as an argument", call)
	}
	if commands.inputs[sends[0]] != text {
		t.Errorf("standard input differs from the pasted text")
	}
}
