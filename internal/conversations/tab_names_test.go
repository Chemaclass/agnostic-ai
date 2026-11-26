package conversations

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ghosttyTabsOutput is what the Ghostty read printed on Ghostty 1.4.0
// for five single-pane tabs, with the names changed.
const ghosttyTabsOutput = "1\x1f/dev/ttys022\x1fcodex:agnostic\x1e1\x1f/dev/ttys010\x1fclaude:agnostic\x1e1\x1f/dev/ttys024\x1fnotes\x1e1\x1f/dev/ttys025\x1fdocs\x1e1\x1f/dev/ttys001\x1f"

func TestParseTabNames_ReadsEachGhosttyTab(t *testing.T) {
	tabs := parseTabNames(ghosttyTabsOutput +
		"\x1e2\x1f/dev/ttys030\x1f/dev/ttys031\x1fsplit" +
		"\x1e1\x1f/dev/ttys040\x1fa\x1fb" +
		"\x1e1\x1fnot a tty\x1fname" +
		"\x1ex\x1f/dev/ttys041\x1fname" +
		"\x1e")

	want := []TerminalTab{
		{Name: "codex:agnostic", TTYs: []string{"/dev/ttys022"}},
		{Name: "claude:agnostic", TTYs: []string{"/dev/ttys010"}},
		{Name: "notes", TTYs: []string{"/dev/ttys024"}},
		{Name: "docs", TTYs: []string{"/dev/ttys025"}},
		{Name: "", TTYs: []string{"/dev/ttys001"}},
		{Name: "split", TTYs: []string{"/dev/ttys030", "/dev/ttys031"}},
	}
	if !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs = %+v, want %+v", tabs, want)
	}
	if parseTabNames("") != nil {
		t.Error("no Ghostty window lists no tab")
	}
}

func TestTabNameReadArgs_CarryNoRuntimeValue(t *testing.T) {
	for app, lines := range tabNameReads {
		args, ok := tabNameReadArgs(app)
		if !ok || !reflect.DeepEqual(args, script(lines...)) || args[len(args)-1] != "end run" {
			t.Errorf("%s: args %q end with a value or are not the fixed script", app, args)
		}
		for _, line := range lines {
			if strings.Contains(line, "argv") {
				t.Errorf("%s: the read takes a value from argv: %q", app, line)
			}
		}
		if lines[0] != `if not (application "`+app+`" is running) then return ""` {
			t.Errorf("%s: the read does not first skip an app that is not running: %q", app, lines[0])
		}
	}
	if _, ok := tabNameReadArgs("Terminal"); ok {
		t.Error("Terminal has no read yet")
	}
}

// tabSync is one live Claude Code chat, alone in Ghostty tab ttys004,
// and another in tab ttys005 of the same folder.
type tabSync struct {
	scanner  *Scanner
	launcher *fakeLauncher
	named    []string
	toolErr  error
}

func newTabSync(t *testing.T) *tabSync {
	t.Helper()
	scanner, stores := testScanner(t, noSQLite, nil)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeClaudeLive(t, stores, idSafari, 45864)
	writeClaudeLive(t, stores, idCache, 45964)
	scanner.Processes = processesWithTable(ghosttyTable(
		ghosttyTab("/dev/ttys004", 45469, 45864, "claude"),
		ghosttyTab("/dev/ttys005", 45569, 45964, "claude"),
	))
	return &tabSync{scanner: scanner, launcher: &fakeLauncher{}}
}

func (s *tabSync) inTool(_ context.Context, c Conversation, title string) error {
	s.named = append(s.named, c.ID+" "+title)
	return s.toolErr
}

// sync scans, as a refresh does, then follows the tabs the read lists.
func (s *tabSync) sync(t *testing.T, tabs ...TerminalTab) []TabRenameFailure {
	t.Helper()
	s.scanner.Scan(context.Background(), ScanOptions{})
	s.launcher.tabs = map[string][]TerminalTab{"Ghostty": tabs}
	return s.scanner.SyncTabNames(context.Background(), s.launcher, s.inTool)
}

func (s *tabSync) title(t *testing.T, id string) string {
	t.Helper()
	return byID(t, s.scanner.Scan(context.Background(), ScanOptions{}), id).Title
}

func tab(name string, ttys ...string) TerminalTab { return TerminalTab{Name: name, TTYs: ttys} }

func TestSyncTabNames_TakesANameTypedOnASinglePaneTab(t *testing.T) {
	s := newTabSync(t)

	failures := s.sync(t, tab("codex:agnostic", "/dev/ttys004"), tab("", "/dev/ttys005"))

	if len(failures) != 0 || strings.Join(s.named, "; ") != idSafari+" codex:agnostic" {
		t.Fatalf("failures %v, tool got %q", failures, s.named)
	}
	if title := s.title(t, idSafari); title != "codex:agnostic" {
		t.Errorf("renamed chat = %q", title)
	}
	if title := s.title(t, idCache); title != "Fix the build" {
		t.Errorf("the other tab's chat = %q", title)
	}
	if calls := strings.Join(s.launcher.calls, "; "); calls != "read Ghostty" {
		t.Errorf("calls = %q, want one read", calls)
	}
}

func TestSyncTabNames_LeavesTheNameAloneOrSaysWhy(t *testing.T) {
	for name, tc := range map[string]struct {
		afterScan func(t *testing.T, s *tabSync)
		tabs      []TerminalTab
		readErr   error
		fails     string
	}{
		"cleared override": {tabs: []TerminalTab{tab("", "/dev/ttys004")}},
		"split tab":        {tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys004", "/dev/ttys006")}},
		"tty in two tabs":  {tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys004"), tab("other", "/dev/ttys004")}},
		"no tab shows it":  {tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys009")}},
		"Automation denied": {
			tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys004")}, readErr: errAutomationDenied,
		},
		"session ended after the scan": {
			afterScan: func(t *testing.T, s *tabSync) {
				t.Helper()
				s.scanner.Processes = fakeProcesses{}
			},
			tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys004")},
		},
		"rename lock held": {
			afterScan: func(t *testing.T, s *tabSync) {
				t.Helper()
				release, err := s.scanner.LockRename(ToolClaude, idSafari)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(release)
			},
			tabs: []TerminalTab{tab("codex:agnostic", "/dev/ttys004")},
		},
		"name too long": {
			tabs:  []TerminalTab{tab(strings.Repeat("x", titleRunes+1), "/dev/ttys004")},
			fails: "the name is longer than 120 characters",
		},
		"name may hold a credential": {
			tabs:  []TerminalTab{tab("token ghp_0123456789abcdefghijABCDEFGHIJ012345", "/dev/ttys004")},
			fails: "the name may hold a password or key, so it is not saved",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTabSync(t)
			s.launcher.readErr = tc.readErr
			s.launcher.tabs = map[string][]TerminalTab{"Ghostty": tc.tabs}
			s.scanner.Scan(context.Background(), ScanOptions{})
			if tc.afterScan != nil {
				tc.afterScan(t, s)
			}
			failures := s.scanner.SyncTabNames(context.Background(), s.launcher, s.inTool)

			var said []string
			for _, failure := range failures {
				said = append(said, failure.Err.Error())
			}
			if strings.Join(said, "; ") != tc.fails || len(s.named) != 0 {
				t.Errorf("failures %q, want %q; tool got %q", said, tc.fails, s.named)
			}
			if again := s.scanner.SyncTabNames(context.Background(), s.launcher, s.inTool); len(again) != 0 || len(s.named) != 0 {
				t.Errorf("a second look tried again: %v, tool got %q", again, s.named)
			}
			s.scanner.Processes = processesWithTable(ghosttyTable(
				ghosttyTab("/dev/ttys004", 45469, 45864, "claude"),
				ghosttyTab("/dev/ttys005", 45569, 45964, "claude"),
			))
			if title := s.title(t, idSafari); title != "Fix the build" {
				t.Errorf("chat = %q", title)
			}
		})
	}
}

func TestSyncTabNames_TriesANameTheToolRefusedOncePerChange(t *testing.T) {
	s := newTabSync(t)
	s.toolErr = &RenameError{Tool: ToolCodex, Reason: RenameNoProgram}

	first := s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
	again := s.sync(t, tab("codex:agnostic", "/dev/ttys004"))

	var notTaken *ToolRenameError
	if len(first) != 1 || !errors.As(first[0].Err, &notTaken) || first[0].Conversation.ID != idSafari || len(again) != 0 {
		t.Fatalf("first %v, again %v", first, again)
	}
	if len(s.named) != 1 || s.title(t, idSafari) != "Fix the build" {
		t.Errorf("tool got %q; chat = %q", s.named, s.title(t, idSafari))
	}
	s.toolErr = nil
	s.sync(t, tab("codex:next", "/dev/ttys004"))
	if s.title(t, idSafari) != "codex:next" {
		t.Errorf("a new name after a refused one: %q", s.title(t, idSafari))
	}
}

func TestSyncTabNames_DoesNotRenameAgainForAnUnchangedName(t *testing.T) {
	s := newTabSync(t)

	for range 3 {
		s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
	}

	if len(s.named) != 1 {
		t.Errorf("tool got %q, want one rename", s.named)
	}
}

func TestSyncTabNames_FirstLookKeepsASavedName(t *testing.T) {
	s := newTabSync(t)
	if err := s.scanner.Rename(ToolClaude, idSafari, "Chosen in the list"); err != nil {
		t.Fatal(err)
	}

	s.sync(t, tab("old tab name", "/dev/ttys004"))
	if len(s.named) != 0 || s.title(t, idSafari) != "Chosen in the list" {
		t.Fatalf("first look: tool got %q, chat = %q", s.named, s.title(t, idSafari))
	}
	s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
	if s.title(t, idSafari) != "codex:agnostic" {
		t.Errorf("a name typed after the first look: %q", s.title(t, idSafari))
	}
}

func TestSyncTabNames_TwoChatsOnOneTTYAreLeftAlone(t *testing.T) {
	s := newTabSync(t)
	table := ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 45864, "claude"))
	table = append(table, ProcessInfo{PID: 45964, PPID: 45470, Stat: "S+", TTY: "/dev/ttys004", Started: "Tue Nov 10 21:49:17 2026", Command: "claude"})
	s.scanner.Processes = processesWithTable(table)

	s.sync(t, tab("codex:agnostic", "/dev/ttys004"))

	if len(s.named) != 0 || len(s.launcher.calls) != 0 {
		t.Errorf("tool got %q, calls %q", s.named, s.launcher.calls)
	}
}

func TestSyncTabNames_ReadsOnlyAppsWithAReadAndAChatInThem(t *testing.T) {
	s := newTabSync(t)
	s.scanner.Processes = fakeProcesses{}

	s.sync(t)

	if len(s.launcher.calls) != 0 {
		t.Errorf("calls %q with no live chat in a tab", s.launcher.calls)
	}
}

func TestSyncTabNames_BothDirectionsInSequence(t *testing.T) {
	s := newTabSync(t)
	s.sync(t, tab("manual", "/dev/ttys004"))
	if s.title(t, idSafari) != "manual" {
		t.Fatalf("manual tab rename: %q", s.title(t, idSafari))
	}

	// F2 saves a newer name; its tab write lands after the next look.
	if _, err := s.scanner.RenameChat(context.Background(), ChatRename{Tool: ToolClaude, ID: idSafari, Title: "from F2", InTool: s.inTool}); err != nil {
		t.Fatal(err)
	}
	s.sync(t, tab("manual", "/dev/ttys004"))
	if s.title(t, idSafari) != "from F2" {
		t.Fatalf("the older tab name undid F2: %q", s.title(t, idSafari))
	}
	s.sync(t, tab("from F2", "/dev/ttys004"))
	s.sync(t, tab("typed again", "/dev/ttys004"))

	want := []string{idSafari + " manual", idSafari + " from F2", idSafari + " typed again"}
	if !reflect.DeepEqual(s.named, want) || s.title(t, idSafari) != "typed again" {
		t.Errorf("tool got %q, chat = %q", s.named, s.title(t, idSafari))
	}
}

func TestRenameChat_LeavesANameSavedMeanwhile(t *testing.T) {
	s := newTabSync(t)
	if err := s.scanner.Rename(ToolClaude, idSafari, "newer"); err != nil {
		t.Fatal(err)
	}
	older := ""

	_, err := s.scanner.RenameChat(context.Background(), ChatRename{Tool: ToolClaude, ID: idSafari, Title: "older", InTool: s.inTool, Saved: &older})

	if !errors.Is(err, errRenamedMeanwhile) || len(s.named) != 0 || s.title(t, idSafari) != "newer" {
		t.Errorf("err %v, tool got %q, chat = %q", err, s.named, s.title(t, idSafari))
	}
}

func TestRemember_KeepsTheTabNameOnlyForTheSameTab(t *testing.T) {
	host := terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}
	name := "codex:agnostic"
	file := tabsFile{Tabs: map[string]rememberedTab{"claude:x": {Host: host, At: "2026-11-01T12:00:00Z", TabName: &name}}}
	now := parseStamp("2026-11-10T12:00:00Z")

	file.remember("claude:x", host, now)
	if kept := file.Tabs["claude:x"].TabName; kept == nil || *kept != name {
		t.Errorf("the same tab lost its name: %v", kept)
	}
	file.remember("claude:x", terminalHost{App: "Ghostty", TTY: "/dev/ttys005"}, now)
	if kept := file.Tabs["claude:x"].TabName; kept != nil {
		t.Errorf("another tab took the name %q", *kept)
	}
}

func TestSyncTabNames_ANewChatDoesNotTakeTheNameAnEarlierChatLeftOnTheTab(t *testing.T) {
	for name, earlier := range map[string]func(t *testing.T, s *tabSync){
		"named with F2 before tab names were read": func(t *testing.T, s *tabSync) {
			t.Helper()
			if err := s.scanner.Rename(ToolClaude, idSafari, "Fix login"); err != nil {
				t.Fatal(err)
			}
			s.scanner.Scan(context.Background(), ScanOptions{})
		},
		"named on the tab": func(t *testing.T, s *tabSync) {
			t.Helper()
			s.sync(t, tab("Fix login", "/dev/ttys004"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTabSync(t)
			s.scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 45864, "claude")))
			earlier(t, s)
			s.named = nil
			// The first chat ends and a second one starts in the same tab.
			s.scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45569, 45964, "claude")))

			s.sync(t, tab("Fix login", "/dev/ttys004"))

			if len(s.named) != 0 || s.title(t, idCache) != "Fix the build" {
				t.Errorf("tool got %q, second chat = %q", s.named, s.title(t, idCache))
			}
			s.sync(t, tab("typed later", "/dev/ttys004"))
			if s.title(t, idCache) != "typed later" {
				t.Errorf("a name typed later: %q", s.title(t, idCache))
			}
		})
	}
}

func TestSyncTabNames_AResumedSessionLooksAtItsTabAfresh(t *testing.T) {
	s := newTabSync(t)
	s.scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 45864, "claude")))
	s.sync(t, tab("first", "/dev/ttys004"))
	if err := s.scanner.Rename(ToolClaude, idSafari, "Chosen"); err != nil {
		t.Fatal(err)
	}
	s.named = nil
	// The session ends and is resumed by a new process in a tab that
	// reuses the tty and still shows another name.
	write(t, filepath.Join(s.scanner.Stores.ClaudeDir, "sessions", "46000.json"), `{"pid":46000,"sessionId":"`+idSafari+`","status":"idle"}`)
	s.scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45969, 46000, "claude")))

	s.sync(t, tab("scratch", "/dev/ttys004"))

	if len(s.named) != 0 || s.title(t, idSafari) != "Chosen" {
		t.Errorf("tool got %q, chat = %q", s.named, s.title(t, idSafari))
	}
}

func TestSyncTabNames_AFolderLimitedServerFollowsNoTab(t *testing.T) {
	s := newTabSync(t)
	s.launcher.tabs = map[string][]TerminalTab{"Ghostty": {tab("codex:agnostic", "/dev/ttys004")}}

	s.scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})
	s.scanner.SyncTabNames(context.Background(), s.launcher, s.inTool)

	if len(s.launcher.calls) != 0 || len(s.named) != 0 {
		t.Errorf("calls %q, tool got %q", s.launcher.calls, s.named)
	}
}

func TestSyncTabNames_KeepsANameSavedWhileTheTabsAreRead(t *testing.T) {
	s := newTabSync(t)
	s.sync(t, tab("manual", "/dev/ttys004"))
	s.launcher.onRead = func() {
		if err := s.scanner.Rename(ToolClaude, idSafari, "from F2"); err != nil {
			t.Error(err)
		}
	}

	s.sync(t, tab("typed", "/dev/ttys004"))

	if !reflect.DeepEqual(s.named, []string{idSafari + " manual"}) || s.title(t, idSafari) != "from F2" {
		t.Errorf("tool got %q, chat = %q", s.named, s.title(t, idSafari))
	}
}

func TestSyncTabNames_PausesReadsAfterARefusalOrATimeout(t *testing.T) {
	for name, readErr := range map[string]error{
		"Automation denied": errAutomationDenied,
		"prompt unanswered": fmt.Errorf("osascript: %w", context.DeadlineExceeded),
	} {
		t.Run(name, func(t *testing.T) {
			s := newTabSync(t)
			now := s.scanner.Now()
			s.scanner.Now = func() time.Time { return now }
			s.launcher.readErr = readErr

			s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
			s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
			if len(s.launcher.calls) != 1 {
				t.Fatalf("calls %q, want one read before the pause ends", s.launcher.calls)
			}
			now = now.Add(tabNameReadPause + time.Second)
			s.launcher.readErr = nil
			s.sync(t, tab("codex:agnostic", "/dev/ttys004"))
			if len(s.launcher.calls) != 2 || s.title(t, idSafari) != "codex:agnostic" {
				t.Errorf("after the pause: calls %q, chat = %q", s.launcher.calls, s.title(t, idSafari))
			}
		})
	}
}
