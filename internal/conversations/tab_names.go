package conversations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TerminalTab is one tab of a terminal app: the name the person gave
// it, empty when none, and one tty per pane, empty when unavailable.
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
		`if (o contains (character id 30)) or (o contains (character id 31)) then set o to ""`,
		"set row to row & (character id 31) & o",
		"set end of rows to row",
		"end repeat",
		"end repeat",
		"end tell",
		"set AppleScript's text item delimiters to (character id 30)",
		"if (count of rows) is 0 then return \"\"",
		"return (rows as text) & (character id 30)",
	},
}

// noTabNameRead are the tab apps whose tab names are not read yet, and
// why. Their tabs still take the name the person gives a chat.
var noTabNameRead = map[string]string{
	"Terminal": "a manual rename and a title the shell sets are not yet told apart on a real host",
	"iTerm":    "which of the tab title and the session name a manual rename changes is not yet checked on a real host",
	"kitty":    "the title_overridden field of kitty @ ls is verified on Linux but not yet on macOS",
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

// tabNameReadPause keeps a failed read from running on every refresh.
const tabNameReadPause = 5 * time.Minute

func canReadTabNames(app, goos string) bool {
	if _, ok := tabNameReads[app]; ok {
		return true
	}
	_, ok := commandTabNameReader(app, goos)
	return ok
}

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

func uniqueTabChats(results []sourceResult) []tabChat {
	var chats []tabChat
	onTTY := map[string]int{}
	for _, result := range results {
		for index, rows := range [][]Conversation{result.conversations, result.unlisted} {
			for _, c := range rows {
				if c.Open.Kind != RouteFocus || c.placed == nil {
					continue
				}
				if len(c.placed.ttys) > 0 {
					for _, tty := range c.placed.ttys {
						onTTY[tty]++
					}
					continue
				}
				if c.placed.host.TTY != "" {
					onTTY[c.placed.host.TTY]++
				}
				host, ok := tabOf(c)
				if index == 0 && ok && !RenameReachesCloud(c) && canReadTabNames(host.App, runtime.GOOS) {
					chats = append(chats, tabChat{c: c, host: host})
				}
			}
		}
	}
	return slices.DeleteFunc(chats, func(chat tabChat) bool { return onTTY[chat.host.TTY] != 1 })
}

func (s *Scanner) keepTabChats(opts ScanOptions, results []sourceResult) {
	var chats []tabChat
	// A folder-limited scan cannot count chats outside its scope that share a tty.
	if len(opts.Within) == 0 {
		chats = uniqueTabChats(results)
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
	if len(kept.chats) == 0 {
		return nil
	}
	// Take the saved names before checking for a rename and reading the tabs.
	marks, _, err := readMarks(s.MarksPath, s.Now())
	if err != nil {
		return nil
	}
	byApp := map[string][]tabChat{}
	for _, chat := range kept.chats {
		if s.Now().Before(s.tabReadsPaused[chat.host.App]) {
			continue
		}
		release, err := s.LockRename(chat.c.Tool, chat.c.ID)
		if err != nil {
			continue
		}
		release()
		byApp[chat.host.App] = append(byApp[chat.host.App], chat)
	}
	var failures []TabRenameFailure
	for _, app := range slices.Sorted(maps.Keys(byApp)) {
		readCtx, cancel := context.WithTimeout(ctx, tabNameReadTimeout)
		tabs, err := launcher.ReadTabNames(readCtx, app)
		cancel()
		if err != nil {
			if s.tabReadsPaused == nil {
				s.tabReadsPaused = map[string]time.Time{}
			}
			s.tabReadsPaused[app] = s.Now().Add(tabNameReadPause)
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
	release, err := s.LockRename(chat.c.Tool, chat.c.ID)
	if errors.Is(err, ErrRenameInProgress) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()
	saved := file.Marks[markKey(chat.c.Tool, chat.c.ID)].Title
	changed, first, sameTab := s.noteTabName(chat, name, saved)
	if !changed || name == "" {
		return nil
	}
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
	if title == saved || !stillRuns(chat.c, s.Processes) {
		return nil
	}
	_, err = s.renameChat(ctx, ChatRename{
		Tool: chat.c.Tool, ID: chat.c.ID, Title: title, Opts: opts, InTool: inTool,
		tab: &chat,
	})
	if errors.Is(err, errTabMoved) {
		return nil
	}
	return err
}

func tabSessionOf(c Conversation) string {
	session := strconv.Itoa(c.process.PID) + "@" + c.process.ProcStart
	// A Codex thread can outlive the terminal client that showed it.
	if client := c.placed.process; client != *c.process {
		session += "|" + strconv.Itoa(client.PID) + "@" + client.ProcStart
	}
	return session
}

func (s *Scanner) resetTabNameObservation(c Conversation, host terminalHost) {
	path := s.tabsPath()
	release, err := lockMarks(path)
	if err != nil {
		return
	}
	defer release()
	file := readTabs(path)
	key := markKey(c.Tool, c.ID)
	tab, ok := file.Tabs[key]
	if !ok || tab.Host != host || tab.NameSession != tabSessionOf(c) || tab.NameFingerprint == "" {
		return
	}
	tab.NameSession = ""
	file.Tabs[key] = tab
	_ = writeJSONFile(path, file)
}

// noteTabName records an observation only if the chat's saved title still
// matches the snapshot taken before reading the terminal.
func (s *Scanner) noteTabName(chat tabChat, name, saved string) (changed, first bool, sameTab []string) {
	key, host, session := markKey(chat.c.Tool, chat.c.ID), chat.host, tabSessionOf(chat.c)
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
		tab.NameFingerprint, tab.NameSession = "", session
	}
	hash := sha256.Sum256([]byte(name))
	fingerprint := hex.EncodeToString(hash[:])
	if tab.NameFingerprint == fingerprint {
		return false, false, nil
	}
	current, err := s.savedName(chat.c.Tool, chat.c.ID)
	if err != nil || current != saved {
		return false, false, nil
	}
	first = tab.NameFingerprint == ""
	inherited := false
	for other, kept := range file.Tabs {
		if other == key || kept.Host != host {
			continue
		}
		sameTab = append(sameTab, other)
		inherited = inherited || kept.NameFingerprint == fingerprint
	}
	tab.NameFingerprint = fingerprint
	file.Tabs[key] = tab
	if writeJSONFile(path, file) != nil {
		return false, false, nil
	}
	if first && inherited {
		return false, true, nil
	}
	return true, first, sameTab
}
