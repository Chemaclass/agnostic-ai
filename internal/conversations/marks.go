package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// State is where a conversation stands for the person: whether it
// waits for them, runs, was finished by hand, or went quiet.
type State string

const (
	// StateBlocked: a live conversation whose tool says it waits for the
	// person, and that the person has not looked at since its last write.
	StateBlocked State = "blocked"
	// StateWorking: a live conversation whose tool says the agent is busy.
	StateWorking State = "working"
	// StateCompleted: the person marked it completed, and since then
	// nothing was written and its tool never said it was working.
	StateCompleted State = "completed"
	// StateUnread: something was written after the person last saw it.
	StateUnread State = "unread"
	// StateActive: open now, or written within activeSpan.
	StateActive State = "active"
	// StateDormant: closed and quiet for longer than activeSpan.
	StateDormant State = "dormant"
)

// activeSpan is how long a closed conversation stays active after its last write.
const activeSpan = 24 * time.Hour

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
	MarkHidden Mark = "hidden"
)

// KnownMark reports whether mark is one the store keeps.
func KnownMark(mark Mark) bool {
	return mark == MarkSeen || mark == MarkCompleted || mark == MarkOpen || mark == MarkHidden
}

const marksSchemaVersion = "conversation-marks/v1"

// maxMarksBytes bounds the marks file: about 100 bytes a conversation.
const maxMarksBytes = 8 << 20

// marksFile is the person's marks on this machine. It holds tool
// names, ids, times and the names the person gave conversations, never
// the conversations' own text.
type marksFile struct {
	SchemaVersion string `json:"schemaVersion"`
	// Since is the baseline: everything written up to it counts as seen.
	// It is when the file began, so the chats written before the person
	// started marking are not all unread.
	Since string               `json:"since"`
	Marks map[string]markEntry `json:"marks"`
}

type markEntry struct {
	SeenAt      string `json:"seenAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	HiddenAt    string `json:"hiddenAt,omitempty"`
	// Title is the name the person chose, shown over the tool's own. A
	// new message keeps it.
	Title string `json:"title,omitempty"`
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

// readMarks loads the marks file. A missing file is a new one that
// begins now; created says so.
func readMarks(path string, now time.Time) (file marksFile, created bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return marksFile{SchemaVersion: marksSchemaVersion, Since: stamp(now), Marks: map[string]markEntry{}}, true, nil
	}
	if err != nil {
		return marksFile{}, false, fmt.Errorf("read conversation marks: %w", err)
	}
	if len(raw) > maxMarksBytes {
		return marksFile{}, false, fmt.Errorf("conversation marks %s is larger than %d bytes", path, maxMarksBytes)
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return marksFile{}, false, fmt.Errorf("conversation marks %s: %w", path, err)
	}
	if file.Marks == nil {
		file.Marks = map[string]markEntry{}
	}
	return file, false, nil
}

// writeMarks replaces the marks file in one step, so a reader never
// sees half of it.
func writeMarks(path string, file marksFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("write conversation marks: %w", err)
	}
	temp, err := os.CreateTemp(dir, "conversation-marks-*")
	if err != nil {
		return fmt.Errorf("write conversation marks: %w", err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(file); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write conversation marks: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write conversation marks: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
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
// every new prompt is, puts it back.
func stateOf(c Conversation, entry markEntry, since, now time.Time) State {
	written := lastWritten(c)
	completedAt := parseStamp(entry.CompletedAt)
	// Seen is the later of the conversation's own mark and the file's baseline.
	seenAt := parseStamp(entry.SeenAt)
	if since.After(seenAt) {
		seenAt = since
	}
	switch {
	case c.Live && c.Activity == ActivityWaiting && (seenAt.IsZero() || written.After(seenAt)):
		return StateBlocked
	case c.Live && c.Activity == ActivityWorking:
		return StateWorking
	case !completedAt.IsZero() && !written.After(completedAt):
		return StateCompleted
	case !seenAt.IsZero() && written.After(seenAt):
		return StateUnread
	case c.Live || now.Sub(written) < activeSpan:
		return StateActive
	default:
		return StateDormant
	}
}

// needsPerson reports whether a state counts toward View.NeedsYou.
func needsPerson(state State) bool {
	return state == StateBlocked || state == StateUnread
}

// applyMarks sets the state and marks of every conversation in
// results, in place, takes out the ones the person hid, and returns how
// many need the person. A completion or a hiding that a later write
// overtook, or whose conversation its tool reported working in a read
// that began at readAt, after the mark, is cleared in the file, so the file holds only those that still stand, and a
// conversation that worked is not completed or hidden again once the
// work ends. The entry keeps its seen mark and chosen name. Without a
// marks path, or with a file that cannot be read, nothing is unread,
// completed or hidden.
func (s *Scanner) applyMarks(results []sourceResult, readAt time.Time) (needsYou int, err error) {
	now := s.Now()
	file := marksFile{Marks: map[string]markEntry{}}
	changed := false
	if s.MarksPath != "" {
		// The read, the changes below and the write are one step for
		// every server that shares the file.
		release, lockErr := lockMarks(s.MarksPath)
		if lockErr == nil {
			defer release()
			read, created, readErr := readMarks(s.MarksPath, now)
			if readErr == nil {
				file, changed = read, created
			}
			err = readErr
		} else {
			err = lockErr
		}
	}
	since := parseStamp(file.Since)
	for r := range results {
		// The rows that stay are written over the start of the same list.
		listed := results[r].conversations[:0]
		for _, c := range results[r].conversations {
			key := markKey(c.Tool, c.ID)
			entry := file.Marks[key]
			written := lastWritten(c)
			working := c.Live && c.Activity == ActivityWorking
			if lifted(entry.CompletedAt, written, working, readAt) {
				entry.CompletedAt = ""
				file.Marks[key] = entry
				changed = true
			}
			if lifted(entry.HiddenAt, written, working, readAt) {
				entry.HiddenAt = ""
				file.Marks[key] = entry
				changed = true
			}
			if entry.HiddenAt != "" {
				continue
			}
			c.renamed = entry.Title
			c.showChosenName()
			c.State = stateOf(c, entry, since, now)
			c.SeenAt = entry.SeenAt
			c.CompletedAt = entry.CompletedAt
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

// overtaken reports whether something was written after a mark made at
// markedAt; an empty markedAt is no mark.
func overtaken(markedAt string, written time.Time) bool {
	at := parseStamp(markedAt)
	return !at.IsZero() && written.After(at)
}

// lifted reports whether a mark made at markedAt no longer stands: a
// later write overtook it, or the conversation was working in a read
// that began at readAt. The tools are read before the marks file is
// locked, so a mark made by another server since then stands. Marks keep
// whole seconds: one made in the second the read began stands too.
func lifted(markedAt string, written time.Time, working bool, readAt time.Time) bool {
	if markedAt == "" {
		return false
	}
	return overtaken(markedAt, written) || working && parseStamp(markedAt).Before(readAt.Truncate(time.Second))
}

// Mark records what the person says about one conversation, as of now.
// Completing or hiding one also marks it seen, so a later write brings
// it back as unread.
func (s *Scanner) Mark(tool Tool, id string, mark Mark) error {
	changes := map[Mark]func(entry *markEntry, now string){
		MarkSeen: func(entry *markEntry, now string) { entry.SeenAt = now },
		MarkCompleted: func(entry *markEntry, now string) {
			entry.SeenAt, entry.CompletedAt = now, now
		},
		MarkOpen: func(entry *markEntry, _ string) { entry.CompletedAt = "" },
		MarkHidden: func(entry *markEntry, now string) {
			entry.SeenAt, entry.HiddenAt = now, now
		},
	}
	change, known := changes[mark]
	if !known {
		return fmt.Errorf("unknown mark %q", mark)
	}
	return s.changeMark(tool, id, change)
}

// changeMark applies change to one conversation's marks and saves them.
func (s *Scanner) changeMark(tool Tool, id string, change func(entry *markEntry, now string)) error {
	if s.MarksPath == "" {
		return errors.New("this server keeps no conversation marks")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	key := markKey(tool, id)
	entry := file.Marks[key]
	change(&entry, stamp(now))
	file.Marks[key] = entry
	return writeMarks(s.MarksPath, file)
}

// Rename saves the name the person chose for one conversation. It
// leaves its seen, completed and hidden marks as they were.
func (s *Scanner) Rename(tool Tool, id, title string) error {
	title, err := CleanTitle(title)
	if err != nil {
		return err
	}
	return s.changeMark(tool, id, func(entry *markEntry, _ string) { entry.Title = title })
}

// Reopen finds one conversation, opens it where it lives, marks it seen
// and takes its completion back: opening it is seeing it, and picking
// it up again. found is false when the scan no longer
// lists it. The open has already happened when the mark is saved, so a
// mark that cannot be saved is said beside the reply, not instead of it.
func (s *Scanner) Reopen(ctx context.Context, tool Tool, id string, opts ScanOptions, launcher Launcher) (reply OpenReply, found bool) {
	conversation, found := s.Find(ctx, tool, id, opts)
	if !found {
		return OpenReply{}, false
	}
	reply = Open(ctx, conversation, s.Processes, launcher)
	if !reply.Opened || s.MarksPath == "" {
		return reply, true
	}
	reopened := func(entry *markEntry, now string) { entry.SeenAt, entry.CompletedAt = now, "" }
	if err := s.changeMark(tool, id, reopened); err != nil {
		reply.Message += " It could not be marked as seen: " + err.Error()
	}
	return reply, true
}
