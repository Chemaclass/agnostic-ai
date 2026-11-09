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
	// StateBlocked: a live conversation whose tool says it waits for the person.
	StateBlocked State = "blocked"
	// StateWorking: a live conversation whose tool says the agent is busy.
	StateWorking State = "working"
	// StateCompleted: the person marked it completed and nothing was written since.
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
	// MarkCompleted: the person is done with it, until something new is written.
	MarkCompleted Mark = "completed"
	// MarkOpen takes a completion back.
	MarkOpen Mark = "open"
)

// KnownMark reports whether mark is one the store keeps.
func KnownMark(mark Mark) bool {
	return mark == MarkSeen || mark == MarkCompleted || mark == MarkOpen
}

const marksSchemaVersion = "conversation-marks/v1"

// maxMarksBytes bounds the marks file: about 100 bytes a conversation.
const maxMarksBytes = 8 << 20

// marksFile is the person's marks on this machine. It holds tool
// names, ids and times, never conversation text.
type marksFile struct {
	SchemaVersion string `json:"schemaVersion"`
	// Since is when the file began. A conversation with no seen mark
	// counts as seen at Since, so the chats written before the person
	// started marking are not all unread.
	Since string               `json:"since"`
	Marks map[string]markEntry `json:"marks"`
}

type markEntry struct {
	SeenAt      string `json:"seenAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
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

// stateOf places one conversation. The first rule that holds wins:
// what the tool reports about a live conversation comes before the
// person's marks, and the marks before its age.
func stateOf(c Conversation, entry markEntry, since, now time.Time) State {
	written := lastWritten(c)
	completedAt := parseStamp(entry.CompletedAt)
	seenAt := parseStamp(entry.SeenAt)
	if seenAt.IsZero() {
		seenAt = since
	}
	switch {
	case c.Live && c.Activity == ActivityWaiting:
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
// results, in place, and returns how many need the person. A completion
// that a later write overtook is deleted from the file, so the file
// holds only completions that still stand. Without a marks path, or
// with a file that cannot be read, nothing is unread or completed.
func (s *Scanner) applyMarks(results []sourceResult) (needsYou int, err error) {
	now := s.Now()
	file := marksFile{Marks: map[string]markEntry{}}
	changed := false
	if s.MarksPath != "" {
		read, created, readErr := readMarks(s.MarksPath, now)
		if readErr == nil {
			file, changed = read, created
		}
		err = readErr
	}
	since := parseStamp(file.Since)
	for _, result := range results {
		for i := range result.conversations {
			c := &result.conversations[i]
			key := markKey(c.Tool, c.ID)
			entry := file.Marks[key]
			completedAt := parseStamp(entry.CompletedAt)
			if !completedAt.IsZero() && lastWritten(*c).After(completedAt) {
				entry.CompletedAt = ""
				file.Marks[key] = entry
				changed = true
			}
			c.State = stateOf(*c, entry, since, now)
			c.SeenAt = entry.SeenAt
			c.CompletedAt = entry.CompletedAt
			if needsPerson(c.State) {
				needsYou++
			}
		}
	}
	if err == nil && changed {
		err = writeMarks(s.MarksPath, file)
	}
	return needsYou, err
}

// Mark records what the person says about one conversation, as of now.
// Completing one also marks it seen, so a later write brings it back
// as unread.
func (s *Scanner) Mark(tool Tool, id string, mark Mark) error {
	if s.MarksPath == "" {
		return errors.New("this server keeps no conversation marks")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	file, _, err := readMarks(s.MarksPath, now)
	if err != nil {
		return err
	}
	key := markKey(tool, id)
	entry := file.Marks[key]
	switch mark {
	case MarkSeen:
		entry.SeenAt = stamp(now)
	case MarkCompleted:
		entry.SeenAt = stamp(now)
		entry.CompletedAt = stamp(now)
	case MarkOpen:
		entry.CompletedAt = ""
	default:
		return fmt.Errorf("unknown mark %q", mark)
	}
	file.Marks[key] = entry
	return writeMarks(s.MarksPath, file)
}

// Reopen finds one conversation, opens it where it lives and marks it
// seen: opening it is seeing it. found is false when the scan no longer
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
	if err := s.Mark(tool, id, MarkSeen); err != nil {
		reply.Message += " It could not be marked as seen: " + err.Error()
	}
	return reply, true
}
