package conversations

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TerminalTab is one tab of a terminal app: the name the person gave
// it, empty when none, and the ttys of its panes.
type TerminalTab struct {
	Name string
	TTYs []string
}

// tabNameReads hold, per app, the AppleScript that lists every tab as
// one row: its pane count, the tty of each pane, then the tab's own
// name override, split by unit separators, with rows split by record
// separators. It skips the read when the app is not running, and takes
// no value from argv.
var tabNameReads = map[string][]string{
	"Ghostty": {
		`if not (application "Ghostty" is running) then return ""`,
		"set rows to {}",
		`tell application "Ghostty"`,
		"repeat with w in windows",
		"repeat with k in tabs of w",
		"set ts to terminals of k",
		"set row to (count of ts) as text",
		"repeat with t in ts",
		"set y to tty of t",
		"if y is missing value then set y to \"\"",
		"set row to row & (character id 31) & y",
		"end repeat",
		"set o to name override of k",
		"if o is missing value then set o to \"\"",
		"set row to row & (character id 31) & o",
		"set end of rows to row",
		"end repeat",
		"end repeat",
		"end tell",
		"set AppleScript's text item delimiters to (character id 30)",
		"return rows as text",
	},
}

// noTabNameRead are the tab apps whose tab names are not read yet, and
// why. Their tabs still take the name the person gives a chat.
var noTabNameRead = map[string]string{
	"Terminal": "a manual rename and a title the shell sets are not yet told apart on a real host",
	"iTerm":    "which of the tab title and the session name a manual rename changes is not yet checked on a real host",
	"WezTerm":  "the tab_title field of wezterm cli list is not yet checked on a real host",
	"kitty":    "the title_overridden field of kitty @ ls is not yet checked on a real host",
	"Konsole":  "a manual rename cannot yet be told apart from the tab title format",
}

// parseTabNames reads the rows a tabNameReads script prints. A row
// whose fields do not add up, such as one whose name holds a
// separator, or that names something other than a device, is left out.
func parseTabNames(out string) []TerminalTab {
	var tabs []TerminalTab
	for row := range strings.SplitSeq(out, "\x1e") {
		fields := strings.Split(row, "\x1f")
		panes, err := strconv.Atoi(fields[0])
		if err != nil || panes < 1 || len(fields) != panes+2 {
			continue
		}
		ttys := fields[1 : panes+1]
		if slices.ContainsFunc(ttys, func(tty string) bool { return !strings.HasPrefix(tty, "/dev/") }) {
			continue
		}
		tabs = append(tabs, TerminalTab{Name: fields[panes+1], TTYs: slices.Clone(ttys)})
	}
	return tabs
}

// tabNameReadTimeout bounds one look at an app's tabs. An unanswered
// Automation prompt would otherwise hold it for launchTimeout.
const tabNameReadTimeout = 3 * time.Second

// tabNameReadPause is how long an app is not read again after macOS
// refused the read or it timed out, as it does while the Automation
// prompt waits for an answer, so refreshes do not stack reads.
const tabNameReadPause = 5 * time.Minute

func tabNameReadArgs(app string) ([]string, bool) {
	read, ok := tabNameReads[app]
	if !ok {
		return nil, false
	}
	return script(read...), true
}

// tabChat is a live chat whose tab name a sync may follow, as the last
// scan found it.
type tabChat struct {
	c    Conversation
	host terminalHost
}

// tabChats is what the last scan found for SyncTabNames.
type tabChats struct {
	opts  ScanOptions
	chats []tabChat
}

// keepTabChats keeps the live chats of the scan's rows that each show
// alone in a tab whose name can be read.
func (s *Scanner) keepTabChats(opts ScanOptions, results []sourceResult) {
	var chats []tabChat
	onTTY := map[terminalHost]int{}
	for _, result := range results {
		for _, c := range result.conversations {
			host, ok := tabOf(c)
			if !ok || c.Open.Kind != RouteFocus || RenameReachesCloud(c) {
				continue
			}
			if _, read := tabNameReads[host.App]; !read {
				continue
			}
			onTTY[host]++
			chats = append(chats, tabChat{c: c, host: host})
		}
	}
	chats = slices.DeleteFunc(chats, func(chat tabChat) bool { return onTTY[chat.host] > 1 })
	// A server limited to some folders cannot see a live chat outside
	// them that shares the tty, so it cannot prove the match; a
	// user-level server, such as the companion's, follows every tab.
	if len(opts.Within) > 0 {
		chats = nil
	}
	opts.Before, opts.Query, opts.Tool, opts.NeedsYou = time.Time{}, "", "", nil
	s.tabChatsMu.Lock()
	defer s.tabChatsMu.Unlock()
	s.tabChats = tabChats{opts: opts, chats: chats}
}

// TabRenameFailure is a tab name a sync did not give its chat.
type TabRenameFailure struct {
	Conversation Conversation
	Err          error
}

// SyncTabNames gives each live chat the name the person typed on its
// tab since the server last looked, through the path a rename from the
// list takes. It looks at each app's tabs once, and only at apps that
// show a chat of the last scan. Only a tab with one pane counts, and
// only a name typed since the last look: a name the server wrote
// itself equals the saved one, and an older name never replaces a
// newer one. A sync already running makes this one do nothing.
func (s *Scanner) SyncTabNames(ctx context.Context, launcher Launcher, inTool RenameInTool) []TabRenameFailure {
	if !s.tabSyncMu.TryLock() {
		return nil
	}
	defer s.tabSyncMu.Unlock()
	s.tabChatsMu.Lock()
	kept := s.tabChats
	s.tabChatsMu.Unlock()
	byApp := map[string][]tabChat{}
	for _, chat := range kept.chats {
		if s.Now().Before(s.tabReadsPaused[chat.host.App]) {
			continue
		}
		byApp[chat.host.App] = append(byApp[chat.host.App], chat)
	}
	if len(byApp) == 0 {
		return nil
	}
	// The saved names are read before the tabs, so a name saved while
	// the tabs are read is newer than the tab's and is kept.
	marks, _, err := readMarks(s.MarksPath, s.Now())
	if err != nil {
		return nil
	}
	var failures []TabRenameFailure
	for _, app := range slices.Sorted(maps.Keys(byApp)) {
		readCtx, cancel := context.WithTimeout(ctx, tabNameReadTimeout)
		tabs, err := launcher.ReadTabNames(readCtx, app)
		cancel()
		if errors.Is(err, errAutomationDenied) || errors.Is(err, context.DeadlineExceeded) {
			if s.tabReadsPaused == nil {
				s.tabReadsPaused = map[string]time.Time{}
			}
			s.tabReadsPaused[app] = s.Now().Add(tabNameReadPause)
		}
		if err != nil {
			continue
		}
		byTTY := map[string]TerminalTab{}
		shown := map[string]int{}
		for _, tab := range tabs {
			for _, tty := range tab.TTYs {
				byTTY[tty] = tab
				shown[tty]++
			}
		}
		for _, chat := range byApp[app] {
			tab, ok := byTTY[chat.host.TTY]
			if !ok || shown[chat.host.TTY] != 1 || len(tab.TTYs) != 1 {
				continue
			}
			if err := s.followTabName(ctx, kept.opts, marks, chat, tab.Name, inTool); err != nil {
				failures = append(failures, TabRenameFailure{Conversation: chat.c, Err: err})
			}
		}
	}
	return failures
}

// errTabMoved says the chat no longer runs alone in the tab it was
// read from.
var errTabMoved = errors.New("the chat left the tab")

// followTabName renames chat after name when the person typed it on
// the tab since the last look. The look is recorded first, whatever
// follows, so a name is tried once per change.
func (s *Scanner) followTabName(ctx context.Context, opts ScanOptions, file marksFile, chat tabChat, name string, inTool RenameInTool) error {
	changed, first, sameTab := s.noteTabName(markKey(chat.c.Tool, chat.c.ID), chat.host, sessionOf(chat.c), name)
	if !changed || name == "" {
		return nil
	}
	saved := file.Marks[markKey(chat.c.Tool, chat.c.ID)].Title
	if name == saved || (first && saved != "") {
		return nil
	}
	// A tab keeps the name of an earlier chat it ran, and macOS reuses
	// ttys, so a first look at that name is not one the person typed.
	if first && slices.ContainsFunc(sameTab, func(key string) bool { return file.Marks[key].Title == name }) {
		return nil
	}
	title, err := CleanTitle(name)
	if err != nil {
		return err
	}
	if !stillRuns(chat.c, s.Processes) {
		return nil
	}
	_, err = s.RenameChat(ctx, ChatRename{
		Tool: chat.c.Tool, ID: chat.c.ID, Title: title, Opts: opts, InTool: inTool, Saved: &saved,
		Check: func(found Conversation) error {
			host, ok := tabOf(found)
			if !ok || host != chat.host || RenameReachesCloud(found) || !stillRuns(found, s.Processes) {
				return errTabMoved
			}
			return nil
		},
	})
	switch {
	case errors.Is(err, ErrRenameInProgress), errors.Is(err, ErrConversationGone), errors.Is(err, errTabMoved), errors.Is(err, errRenamedMeanwhile):
		return nil
	default:
		return err
	}
}

// sessionOf names the session process of a live chat.
func sessionOf(c Conversation) string {
	return strconv.Itoa(c.process.PID) + "@" + c.process.ProcStart
}

// noteTabName records name as the one the tab of key showed, under the
// lock every server shares, so two servers never both act on one
// change. changed is false when the last look saw the same name; first
// is true when no look at this tab by this session was recorded before. sameTab are
// the other chats remembered in that tab; a first look whose name one
// of them saw last is not changed.
func (s *Scanner) noteTabName(key string, host terminalHost, session, name string) (changed, first bool, sameTab []string) {
	path := s.tabsPath()
	if path == "" {
		return false, false, nil
	}
	release, err := lockMarks(path)
	if err != nil {
		return false, false, nil
	}
	defer release()
	file := readTabs(path)
	tab, ok := file.Tabs[key]
	if !ok || tab.Host != host {
		tab = rememberedTab{Host: host, At: stamp(s.Now())}
	}
	if tab.NameSession != session {
		tab.TabName, tab.NameSession = nil, session
	}
	if tab.TabName != nil && *tab.TabName == name {
		return false, false, nil
	}
	first = tab.TabName == nil
	inherited := false
	for other, kept := range file.Tabs {
		if other == key || kept.Host != host {
			continue
		}
		sameTab = append(sameTab, other)
		inherited = inherited || (kept.TabName != nil && *kept.TabName == name)
	}
	tab.TabName = &name
	file.Tabs[key] = tab
	if writeJSONFile(path, file) != nil {
		return false, false, nil
	}
	if first && inherited {
		return false, true, nil
	}
	return true, first, sameTab
}
