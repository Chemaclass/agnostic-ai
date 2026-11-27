package conversations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is where a conversation stands for the person: whether it
// waits for them, runs, was finished by hand, or went quiet.
type State string

const (
	// StateBlocked: a live conversation whose tool says it waits for the
	// person, and that the person has not looked at since its last write.
	StateBlocked State = "blocked"
	// StateWorking: a live conversation whose tool says the agent is busy,
	// or one with working subagents that is not known closed.
	StateWorking State = "working"
	// StateCompleted: the person marked it completed, and since then
	// nothing was written and its tool never said it was working.
	StateCompleted State = "completed"
	// StateUnread: something was written after the person last saw it,
	// in a conversation not known closed.
	StateUnread State = "unread"
	// StateActive: open now; a desktop, chat, Cowork or cloud
	// conversation nobody archived, whatever its age; or written within
	// activeSpan when the scan cannot tell whether it is closed.
	StateActive State = "active"
	// StateDormant: a terminal session no process holds, as a full process
	// read showed; archived in its own app; or an editor panel, background
	// job or unchecked terminal session quiet for longer than activeSpan.
	// It has no read state.
	StateDormant State = "dormant"
)

// activeSpan is how long an editor panel, a background job, or a
// terminal session whose process read failed stays active after its
// last write. No reliable signal says when those close.
const activeSpan = 24 * time.Hour

// openUntilArchived are the apps whose conversations stay open until
// the person archives them, here or in the app. Quitting the app closes
// none of them, so their age says nothing.
var openUntilArchived = map[App]bool{AppDesktop: true, AppChat: true, AppCowork: true, AppCloud: true}

// Mark is what the person says about a conversation.
type Mark string

const (
	// MarkSeen: the person has read it up to now.
	MarkSeen Mark = "seen"
	// MarkCompleted: the person is done with it, until something new is
	// written or its tool says it is working.
	MarkCompleted Mark = "completed"
	// MarkOpen takes a completion back.
	MarkOpen Mark = "open"
	// MarkHidden: the person is done with it and wants it out of every
	// list and search, until something new is written or its tool says
	// it is working. The tool's own store keeps the conversation.
	MarkHidden   Mark = "hidden"
	MarkPinned   Mark = "pinned"
	MarkUnpinned Mark = "unpinned"
)

// KnownMark reports whether mark is one the store keeps.
func KnownMark(mark Mark) bool {
	_, ok := markChanges[mark]
	return ok
}

const marksSchemaVersion = "conversation-marks/v1"

// maxMarksBytes bounds the marks file: about 100 bytes a conversation.
const maxMarksBytes = 8 << 20

// marksFile is the person's marks on this machine. It holds tool
// names, ids, times, the names the person gave conversations and the
// paths of their stores and files, never the conversations' own text. A
// Claude Code project folder's name spells the folder the session ran
// in, and a Codex rollout outside CODEX_HOME is kept as a full path.
type marksFile struct {
	SchemaVersion string `json:"schemaVersion"`
	// Since is the baseline: everything written up to it counts as seen.
	// It is when the file began, so the chats written before the person
	// started marking are not all unread.
	Since string               `json:"since"`
	Marks map[string]markEntry `json:"marks"`
	// HistorySince is the first successful read of a source added after marks already existed.
	HistorySince map[historySource]time.Time `json:"historySince"`
	// Stores maps each store a mark names to its folder; see pruneMarks.
	Stores map[string]string `json:"stores,omitempty"`

	extra unknownJSON
}

type historySource string

const (
	historyClaudeChat         historySource = "claude-chat"
	historyClaudeCoworkLocal  historySource = "claude-cowork-local"
	historyClaudeCoworkRemote historySource = "claude-cowork-remote"
	historyChatGPT            historySource = "chatgpt"
)

type historyRead struct {
	source historySource
	at     time.Time
}

func historyOf(c Conversation) historySource {
	switch {
	case c.Tool == ToolClaude && c.App == AppChat:
		return historyClaudeChat
	case c.Tool == ToolClaude && c.App == AppCowork && strings.HasPrefix(c.ID, "local_"):
		return historyClaudeCoworkLocal
	case c.Tool == ToolClaude && c.App == AppCowork:
		return historyClaudeCoworkRemote
	case c.Tool == ToolChatGPT:
		return historyChatGPT
	default:
		return ""
	}
}

func (f *marksFile) recordHistory(results []sourceResult) (changed bool) {
	for _, result := range results {
		for _, read := range result.history {
			if _, known := f.HistorySince[read.source]; known {
				continue
			}
			f.HistorySince[read.source] = read.at.UTC()
			changed = true
		}
	}
	return changed
}

func (f marksFile) baseline(c Conversation, entry markEntry, since time.Time) time.Time {
	if entry.SeenAt != "" || entry.CompletedAt != "" || c.Live && c.Activity == ActivityWaiting {
		return since
	}
	if first := f.HistorySince[historyOf(c)]; first.After(since) {
		return first
	}
	return since
}

type markEntry struct {
	SeenAt      string `json:"seenAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	HiddenAt    string `json:"hiddenAt,omitempty"`
	PinnedAt    string `json:"pinnedAt,omitempty"`
	// ClosedWrittenAt is the last write a scan read while the session was
	// known closed and not seen. A manual resume does not make that write
	// new again; a later one is.
	ClosedWrittenAt string `json:"closedWrittenAt,omitempty"`
	// TwinShownAt, on a cloud session, is its last write that a local
	// session it was shared with showed. The cloud row on its own is
	// unread only for a later write.
	TwinShownAt string `json:"twinShownAt,omitempty"`
	// Title is the name the person chose, shown over the tool's own. A
	// new message keeps it.
	Title string `json:"title,omitempty"`
	// Source, Store and File say which store a complete read listed the
	// conversation in and where its own data sits there; MissingSince and
	// LastMissAt date the complete reads of that store that missed it.
	// See pruneMarks.
	Source       markSource `json:"source,omitempty"`
	Store        string     `json:"store,omitempty"`
	File         string     `json:"file,omitempty"`
	MissingSince string     `json:"missingSince,omitempty"`
	LastMissAt   string     `json:"lastMissAt,omitempty"`

	extra unknownJSON
}

func markKey(tool Tool, id string) string { return string(tool) + ":" + id }

// DefaultMarksPath is the marks file in the user's configuration
// folder, outside every repository. Empty when the folder is unknown.
func DefaultMarksPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agnostic-ai", "conversation-marks.json")
}

// readMarks loads the marks file. A missing file uses since as its baseline.
func readMarks(path string, since time.Time) (file marksFile, created bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return marksFile{
			SchemaVersion: marksSchemaVersion, Since: stamp(since),
			Marks: map[string]markEntry{}, HistorySince: map[historySource]time.Time{},
		}, true, nil
	}
	if err != nil {
		return marksFile{}, false, fmt.Errorf("read conversation marks: %w", err)
	}
	if len(raw) > maxMarksBytes {
		return marksFile{}, false, fmt.Errorf("conversation marks %s is larger than %d bytes", path, maxMarksBytes)
	}
	if err := decodeMarks(raw, &file); err != nil {
		return marksFile{}, false, fmt.Errorf("conversation marks %s: %w", path, err)
	}
	if file.Marks == nil {
		file.Marks = map[string]markEntry{}
	}
	if file.HistorySince == nil {
		// These sources first shipped in v0.96.0. Keep unmarked replies that could already have been listed.
		firstRelease := time.Date(2026, 11, 20, 22, 12, 23, 0, time.UTC)
		file.HistorySince = map[historySource]time.Time{
			historyClaudeChat: firstRelease, historyClaudeCoworkLocal: firstRelease,
			historyClaudeCoworkRemote: firstRelease, historyChatGPT: firstRelease,
		}
	}
	return file, false, nil
}

// writeMarks replaces the marks file in one step, so a reader never
// sees half of it.
func writeMarks(path string, file marksFile) error {
	if err := writeJSONFile(path, file); err != nil {
		return fmt.Errorf("write conversation marks: %w", err)
	}
	return nil
}

func stamp(at time.Time) string { return at.UTC().Truncate(time.Second).Format(time.RFC3339) }

// parseStamp reads a stored time; an empty or unreadable one is the zero time.
func parseStamp(value string) time.Time {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return at
}

// stateOf places one conversation. The first rule that holds wins.
// A tool that says its conversation is busy is believed, and applyMarks
// clears its completion, so it is not completed again once idle. A tool
// that says it waits for the person is believed until the person has looked:
// like a new reply, a wait needs the person only while nothing shows
// they saw it since its last write. So opening or archiving a waiting
// conversation takes it out of the count, and the next write, which
// every new prompt is, puts it back. A conversation its own app archived
// is dormant, the state of a closed one, while that archive holds.
// A terminal session no process holds, by a full process read, is
// archived even with a reply not seen.
// A write read while a session was closed needs nobody, even after a
// resume or a failed process read. Other writes go by the seen mark.
func stateOf(c Conversation, entry markEntry, since, now time.Time) State {
	written := lastWritten(c)
	completedAt := parseStamp(entry.CompletedAt)
	seenAt := seenSince(entry, since)
	attendedAt := seenAt
	if closedWrittenAt := parseStamp(entry.ClosedWrittenAt); closedWrittenAt.After(attendedAt) {
		attendedAt = closedWrittenAt
	}
	switch {
	case c.shellOnly:
		return StateBlocked
	case c.Live && c.Activity == ActivityWaiting && (attendedAt.IsZero() || written.After(attendedAt)):
		return StateBlocked
	case workingNow(c):
		return StateWorking
	case c.underAppArchive:
		return StateDormant
	case !completedAt.IsZero() && !written.After(completedAt):
		return StateCompleted
	case knownClosed(c):
		return StateDormant
	case !attendedAt.IsZero() && written.After(attendedAt):
		return StateUnread
	case c.Live || (!c.liveRead && (openUntilArchived[c.App] || now.Sub(written) < activeSpan)):
		return StateActive
	default:
		return StateDormant
	}
}

// seenSince is the latest of a conversation's own seen mark, the
// file's baseline and, for a cloud session, the last write its local
// twin showed.
func seenSince(entry markEntry, since time.Time) time.Time {
	seenAt := parseStamp(entry.SeenAt)
	if shown := parseStamp(entry.TwinShownAt); shown.After(seenAt) {
		seenAt = shown
	}
	if since.After(seenAt) {
		return since
	}
	return seenAt
}

// unseen reports whether something was written after the person last
// saw the conversation, whatever its state.
func unseen(c Conversation, entry markEntry, since time.Time) bool {
	seenAt := seenSince(entry, since)
	return !seenAt.IsZero() && lastWritten(c).After(seenAt)
}

// unreadMarker: an unseen write on an active row, which does not count
// toward NeedsYou: a session resumed by hand whose reply came while it
// was closed, until the person reads it. An archived row has no read state.
func unreadMarker(c Conversation, unseen bool) bool {
	return unseen && c.Live && c.State == StateActive
}

// knownClosed: a full process read found no process that holds this
// terminal session. A failed or partial read proves nothing.
func knownClosed(c Conversation) bool {
	return c.liveRead && !c.Live
}

func workingNow(c Conversation) bool {
	return (c.Live && c.Activity == ActivityWorking && !c.shellOnly) || (!knownClosed(c) && c.WorkingSubagents != nil && *c.WorkingSubagents > 0)
}

// needsPerson reports whether a state counts toward View.NeedsYou.
func needsPerson(state State) bool {
	return state == StateBlocked || state == StateUnread
}

// applyMarks sets the state and marks of every conversation in
// results, in place, takes out the ones the person hid and the empty
// ones the person did not name, and returns how many need the person. A completion or a hiding that a later write
// overtook, or whose conversation its tool reported working in a read
// that began after the mark, is cleared in the file, so the file holds only those that still stand, and a
// conversation that worked is not completed or hidden again once the
// work ends. The entry keeps its seen mark and chosen name. Without a
// marks path, or with a file that cannot be read, nothing is unread,
// completed or hidden.
func (s *Scanner) applyMarks(results []sourceResult, startedAt time.Time, scanFinished bool) (needsYou int, err error) {
	now := s.Now()
	file := marksFile{Marks: map[string]markEntry{}}
	changed := false
	if s.MarksPath != "" {
		// The read, the changes below and the write are one step for
		// every server that shares the file.
		release, lockErr := lockMarks(s.MarksPath)
		if lockErr == nil {
			defer release()
			read, created, readErr := readMarks(s.MarksPath, startedAt)
			if readErr == nil {
				file, changed = read, created
			}
			err = readErr
		} else {
			err = lockErr
		}
	}
	if err == nil && s.MarksPath != "" {
		if scanFinished && file.recordHistory(results) {
			changed = true
		}
		if s.pruneMarks(&file, results, now) {
			changed = true
		}
	}
	initialSince := parseStamp(file.Since)
	for r := range results {
		// The rows that stay are written over the start of the same list.
		listed := results[r].conversations[:0]
		for _, c := range results[r].conversations {
			key := markKey(c.Tool, c.ID)
			entry := file.Marks[key]
			written := lastWritten(c)
			working := workingNow(c)
			if lifted(entry.CompletedAt, written, working, c.readAt) {
				entry.CompletedAt = ""
				file.Marks[key] = entry
				changed = true
			}
			if lifted(entry.HiddenAt, written, working, c.readAt) {
				entry.HiddenAt = ""
				file.Marks[key] = entry
				changed = true
			}
			if entry.HiddenAt != "" || (c.untyped && entry.Title == "") {
				continue
			}
			entry = withTwinSeen(file, c, entry)
			since := file.baseline(c, entry, initialSince)
			if s.MarksPath != "" && showTwin(file, c, entry, since) {
				changed = true
			}
			if s.MarksPath != "" && knownClosed(c) && unseen(c, entry, since) && written.After(parseStamp(entry.ClosedWrittenAt)) {
				entry.ClosedWrittenAt = written.UTC().Format(time.RFC3339Nano)
				file.Marks[key] = entry
				changed = true
			}
			c.renamed = entry.Title
			c.showChosenName()
			c.State = stateOf(c, entry, since, now)
			c.Unread = unreadMarker(c, unseen(c, entry, since))
			c.SeenAt = entry.SeenAt
			c.CompletedAt = entry.CompletedAt
			c.PinnedAt = visiblePin(file, c, entry)
			if c.State == StateCompleted || c.State == StateDormant {
				c.DeleteUnsupported = s.deleteUnsupported(c)
				if c.DeleteUnsupported == "" && deleteNeedsClosure(c, entry, since, now) {
					c.DeleteUnsupported = deleteNeedsClosureMessage
				}
			}
			if needsPerson(c.State) {
				needsYou++
			}
			listed = append(listed, c)
		}
		results[r].conversations = listed
	}
	if err == nil && changed {
		err = writeMarks(s.MarksPath, file)
	}
	return needsYou, err
}

// appArchive is what the scanner remembers of one conversation its own
// app keeps archived.
type appArchive struct {
	// since is when the archive began: the app's own time when it keeps
	// one, else the last write the scanner saw when it first read the
	// conversation as archived.
	since time.Time
	// lifted: a write after since, or work, took it out of Archived.
	// It stays out while the app keeps it archived.
	lifted bool
}

// followAppArchives sets underAppArchive on every row of results. Every
// read goes through it, the scan and the lookups behind open, mark,
// rename and delete alike, so the scanner sees each change of an app's
// flag, however short.
func (s *Scanner) followAppArchives(results []sourceResult) {
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			c.underAppArchive = s.stillAppArchived(markKey(c.Tool, c.ID), *c, lastWritten(*c), workingNow(*c))
		}
	}
}

// stillAppArchived reports whether a conversation shows under Archived
// because its own app archived it. Like a completion, the archive holds
// until something is written after it or its tool says it is working.
// Writes are compared in whole seconds, since an app may keep the
// archive's time in whole seconds. The scanner remembers each archive
// in memory and never in the marks file, so un-archiving in the app
// brings the conversation back on the next read; a new server begins
// again from what the app says.
func (s *Scanner) stillAppArchived(key string, c Conversation, written time.Time, working bool) bool {
	if !c.appArchived {
		delete(s.appArchives, key)
		return false
	}
	if s.appArchives == nil {
		s.appArchives = map[string]appArchive{}
	}
	archive, known := s.appArchives[key]
	archivedAgain := !c.appArchivedAt.IsZero() && !c.appArchivedAt.Equal(archive.since)
	if !known || archivedAgain {
		archive = appArchive{since: archiveStart(c, written)}
	}
	if written.Truncate(time.Second).After(archive.since) || working {
		archive.lifted = true
	}
	s.appArchives[key] = archive
	return !archive.lifted
}

// archiveStart is when an app archive began: the app's own time when it
// keeps one, else the conversation's last write as read now.
func archiveStart(c Conversation, written time.Time) time.Time {
	if c.appArchivedAt.IsZero() {
		return written
	}
	return c.appArchivedAt
}

func visiblePin(file marksFile, c Conversation, entry markEntry) string {
	if c.Parent != nil {
		return ""
	}
	pin := parseStamp(entry.PinnedAt)
	if pin.IsZero() && c.Cloud != nil {
		pin = parseStamp(file.Marks[markKey(c.Tool, c.Cloud.ID)].PinnedAt)
	}
	if pin.IsZero() {
		return ""
	}
	return pin.UTC().Format(time.RFC3339Nano)
}

func validPinTime(value string) bool {
	return value != "" && !parseStamp(value).IsZero()
}

// withTwinSeen gives a shared row the later seen mark of its own id and
// its cloud id, so a mark on either counts for both.
func withTwinSeen(file marksFile, c Conversation, entry markEntry) markEntry {
	if c.Cloud != nil {
		entry.SeenAt = laterStamp(entry.SeenAt, file.Marks[markKey(c.Tool, c.Cloud.ID)].SeenAt)
	}
	return entry
}

// showTwin saves, on a shared row's cloud id, the cloud write the row
// shows, so the cloud row does not count it again once it shows on its
// own. A closed row saves a write past the one read while the session
// ran only once the person saw it: it may be a reply from the cloud
// side, which the local row no longer shows once the two show apart.
// It reports whether the file changed.
func showTwin(file marksFile, c Conversation, entry markEntry, since time.Time) bool {
	if c.Cloud == nil {
		return false
	}
	key := markKey(c.Tool, c.Cloud.ID)
	twin := file.Marks[key]
	shown := lastWritten(*c.Cloud.row)
	if !c.Live && shown.After(c.Cloud.liveWritten) && shown.After(seenSince(entry, since)) {
		shown = c.Cloud.liveWritten
	}
	if !shown.After(parseStamp(twin.TwinShownAt)) {
		return false
	}
	twin.TwinShownAt = shown.UTC().Format(time.RFC3339Nano)
	file.Marks[key] = twin
	return true
}

// laterStamp is the later of two marks; an empty one is never later.
func laterStamp(a, b string) string {
	if parseStamp(b).After(parseStamp(a)) {
		return b
	}
	return a
}

// overtaken reports whether something was written after a mark made at
// markedAt; an empty markedAt is no mark.
func overtaken(markedAt string, written time.Time) bool {
	at := parseStamp(markedAt)
	return !at.IsZero() && written.After(at)
}

// lifted reports whether a mark made at markedAt no longer stands: a
// later write overtook it, or the conversation was working in a read
// that began at readAt. A row can come from a read older than the scan,
// and the tools are read before the marks file is locked, so a mark made
// since that read stands. Marks keep whole seconds: one made in the
// second the read began stands too.
func lifted(markedAt string, written time.Time, working bool, readAt time.Time) bool {
	if markedAt == "" {
		return false
	}
	return overtaken(markedAt, written) || working && parseStamp(markedAt).Before(readAt.Truncate(time.Second))
}

type markChange func(entry *markEntry, now time.Time)

var markChanges = map[Mark]markChange{
	MarkSeen: func(entry *markEntry, now time.Time) { entry.SeenAt = stamp(now) },
	MarkCompleted: func(entry *markEntry, now time.Time) {
		at := stamp(now)
		entry.SeenAt, entry.CompletedAt = at, at
	},
	MarkOpen: func(entry *markEntry, _ time.Time) { entry.CompletedAt = "" },
	MarkHidden: func(entry *markEntry, now time.Time) {
		at := stamp(now)
		entry.SeenAt, entry.HiddenAt, entry.PinnedAt = at, at, ""
	},
	MarkPinned: func(entry *markEntry, now time.Time) {
		if !validPinTime(entry.PinnedAt) {
			entry.PinnedAt = now.UTC().Format(time.RFC3339Nano)
		}
	},
	MarkUnpinned: func(entry *markEntry, _ time.Time) { entry.PinnedAt = "" },
}

// Mark records what the person says about one conversation, as of now.
// Completing or hiding one also marks it seen, so a later write brings
// it back as unread.
func (s *Scanner) Mark(tool Tool, id string, mark Mark) error {
	change, known := markChanges[mark]
	if !known {
		return fmt.Errorf("unknown mark %q", mark)
	}
	twinChange := change
	if mark == MarkHidden {
		twinChange = markChanges[MarkUnpinned]
	}
	return s.changeMark(tool, id, change, twinChange)
}

type markUpdate struct {
	key    string
	change markChange
}

// changeMark saves a conversation's marks. A linked cloud change is saved in the same update.
func (s *Scanner) changeMark(tool Tool, id string, change, twinChange markChange) error {
	if s.MarksPath == "" {
		return errors.New("this server keeps no conversation marks")
	}
	s.lock()
	defer s.unlock()
	key := markKey(tool, id)
	updates := []markUpdate{{key, change}}
	if cloud := s.sharedClouds[key]; twinChange != nil && cloud != "" {
		updates = append(updates, markUpdate{markKey(tool, cloud), twinChange})
	}
	return s.changeMarks(updates)
}

// changeMarks applies and saves the updates under the marks file's lock.
func (s *Scanner) changeMarks(updates []markUpdate) error {
	// Another server may change the same file: the lock spans the read
	// and the write, so neither discards the other's mark.
	release, err := lockMarks(s.MarksPath)
	if err != nil {
		return err
	}
	defer release()
	now := s.Now()
	file, _, err := readMarks(s.MarksPath, now)
	if err != nil {
		return err
	}
	for _, update := range updates {
		entry := file.Marks[update.key]
		update.change(&entry, now)
		s.attributeMark(&file, update.key, &entry)
		file.Marks[update.key] = entry
	}
	return writeMarks(s.MarksPath, file)
}

// Rename saves the name the person chose for one conversation. It
// leaves its seen, completed and hidden marks as they were.
func (s *Scanner) Rename(tool Tool, id, title string) error {
	title, err := CleanTitle(title)
	if err != nil {
		return err
	}
	return s.changeMark(tool, id, func(entry *markEntry, _ time.Time) { entry.Title = title }, nil)
}

// Reopen finds one conversation, opens it where it lives, marks it seen
// and takes its completion back: opening it is seeing it, and picking
// it up again. found is false when the scan no longer
// lists it. The open has already happened when the mark is saved, so a
// mark that cannot be saved is said beside the reply, not instead of it.
// Neither the lookup nor the mark waits for a running scan to end. A
// second open of the same chat while one runs starts nothing: it waits
// for the first and gets its reply.
func (s *Scanner) Reopen(ctx context.Context, tool Tool, id string, opts ScanOptions, launcher Launcher) (OpenReply, bool) {
	return s.ReopenHelper(ctx, tool, id, "", opts, launcher)
}

// ReopenHelper reopens the chat a Claude Code subagent runs in, as
// Reopen does, and names that subagent in the reply. A subagent with a
// terminal pane of its own, or inside one with a pane, gets that pane
// instead. A subagent the chat no longer lists has finished: its chat
// still opens. An empty subagent is Reopen. Opens of one chat share one
// open, whichever subagents they name; the first one's subagent picks
// whether a pane comes to the front.
func (s *Scanner) ReopenHelper(ctx context.Context, tool Tool, id, subagent string, opts ScanOptions, launcher Launcher) (OpenReply, bool) {
	key := markKey(tool, id)
	if _, noRun := launcher.(noRunLauncher); noRun {
		// An open that may not run a command answers differently, so a
		// proven open never joins one, nor the other way round.
		key += "\x00no-run"
	}
	s.openingMu.Lock()
	if running, ok := s.opening[key]; ok {
		s.openingMu.Unlock()
		<-running.done
		return running.outcome.replyFor(subagent)
	}
	if s.opening == nil {
		s.opening = map[string]*sharedOpen{}
	}
	this := &sharedOpen{done: make(chan struct{}), outcome: openOutcome{reply: OpenReply{Message: "The conversation could not be opened. Try again."}, found: true}}
	s.opening[key] = this
	s.openingMu.Unlock()
	defer func() {
		s.openingMu.Lock()
		delete(s.opening, key)
		s.openingMu.Unlock()
		close(this.done)
	}()
	this.outcome = s.reopen(ctx, tool, id, subagent, opts, launcher)
	return this.outcome.replyFor(subagent)
}

// sharedOpen is one open in progress; done closes once outcome holds
// its answer.
type sharedOpen struct {
	done    chan struct{}
	outcome openOutcome
}

// openOutcome is how one open of a chat ended, which each caller that
// asked for it reads with the subagent it named.
type openOutcome struct {
	reply OpenReply
	found bool
	// chat is the conversation the open found.
	chat Conversation
	// pane is the helper whose own pane came to the front, instead of
	// the chat; empty when the chat did.
	pane string
	// attach is the command that shows the pane of helper attachFor,
	// which no terminal shows.
	attach, attachFor string
	// unmarked is why the opened chat could not be marked seen.
	unmarked string
}

func (o openOutcome) replyFor(subagent string) (OpenReply, bool) {
	if !o.found {
		return OpenReply{}, false
	}
	reply := o.reply
	if subagent != "" {
		picked, listed := subagentIn(o.chat.Helpers, subagent)
		// A joined open may have shown another helper's pane: only the
		// open's own target counts for this one.
		target, _ := paneTarget(o.chat.Helpers, subagent)
		pane, attach := "", ""
		if target.ID != "" && target.ID == o.pane {
			pane = o.pane
		}
		if target.ID != "" && target.ID == o.attachFor {
			attach = o.attach
		}
		switch {
		case reply.Opened && o.pane != "" && pane == "":
			// Another helper's pane is in front, not the chat.
			reply.Reached = reachedParent
			if listed {
				reply.Helper = &picked
				if name := picked.shownName(); name != "" {
					reply.Message += fmt.Sprintf(" Its subagent is '%s'.", name)
				}
			} else {
				reply.Message += " That subagent has finished."
			}
		case !listed && reply.Opened:
			reply.Message += " " + subagentFinished
			reply.Reached = reachedParent
		case listed && reply.Opened:
			reply = subagentReply(reply, o.chat, picked, pane, attach)
		}
	}
	if o.unmarked != "" {
		reply.Message += " It could not be marked as seen: " + o.unmarked
	}
	return reply, true
}

func (s *Scanner) reopen(ctx context.Context, tool Tool, id, subagent string, opts ScanOptions, launcher Launcher) openOutcome {
	conversation, found := s.findToOpen(ctx, tool, id, opts)
	if !found {
		return openOutcome{}
	}
	out := openOutcome{found: true, chat: conversation}
	if pane, ok := paneTarget(conversation.Helpers, subagent); ok {
		opening := openTeammatePane(ctx, conversation, pane.member, s.Processes, launcher)
		if opening.shown {
			out.reply, out.pane = opening.reply, pane.ID
		}
		if opening.attach != "" {
			out.attach, out.attachFor = opening.attach, pane.ID
		}
	}
	if out.pane == "" {
		out.reply = Open(ctx, conversation, s.Processes, launcher)
	}
	if conversation.subagent != "" && conversation.Open.Kind == RouteFocus && out.reply.Opened {
		out.reply.Message += fmt.Sprintf(" Type /subagents to switch to '%s'.", conversation.subagent)
		out.reply.Reached = reachedParent
	}
	if !out.reply.Opened || out.reply.Guessed || s.MarksPath == "" {
		return out
	}
	// A shared row's cloud id gets the same marks, and so does the local
	// row that holds a cloud row opened on its own, so each says the same
	// once the other shows apart.
	reopened := func(entry *markEntry, now time.Time) { entry.SeenAt, entry.CompletedAt = stamp(now), "" }
	updates := []markUpdate{{markKey(tool, id), reopened}}
	if conversation.Cloud != nil && conversation.ID == id {
		updates = append(updates, markUpdate{markKey(tool, conversation.Cloud.ID), reopened})
	}
	if conversation.heldBy != "" {
		updates = append(updates, markUpdate{markKey(tool, conversation.heldBy), reopened})
	}
	if err := s.changeMarks(updates); err != nil {
		out.unmarked = err.Error()
	}
	return out
}
