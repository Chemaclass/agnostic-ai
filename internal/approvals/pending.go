package approvals

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// MaxPendingInput bounds the input a pending request shows. A larger
// one gets no record, so the panel never shows a request cut short.
const MaxPendingInput = 8 << 10

// LiveFresh is how recent companion.live must be for the hook to hold.
// The server touches it every LiveEvery.
const (
	LiveFresh = 15 * time.Second
	LiveEvery = 5 * time.Second
)

// What a click in the companion decides for a pending request.
const (
	DecideApprove = "approve"
	DecideDeny    = "deny"
	// DecideRelease leaves the request to the tool's own prompt.
	DecideRelease = "release"
)

// Decisions are the values a decision file accepts.
var Decisions = []string{DecideApprove, DecideDeny, DecideRelease}

// Pending is a request the approvals hook holds for an answer from the
// companion. It lives in <id>.json in the pending folder.
type Pending struct {
	ID      string       `json:"id"`
	Tool    string       `json:"tool"`
	Session string       `json:"session"`
	Cwd     string       `json:"cwd"`
	Call    string       `json:"call"`
	Input   PendingInput `json:"input"`
	// Digest is SHA-256 of the tool, session, call and the full input
	// the hook read; the hook acts only on a decision with this digest.
	Digest  string    `json:"digest"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	PID     int       `json:"pid"`
}

// PendingInput is the part of a call's input the panel shows.
type PendingInput = conversations.PendingInput

// PendingDecision is the companion's answer, in <id>.decision.json.
type PendingDecision struct {
	Decision string `json:"decision"`
	Digest   string `json:"digest"`
}

// PendingDir is the folder of held requests in the agnostic-ai home.
func PendingDir(agnosticHome string) string {
	return filepath.Join(agnosticHome, "local", "approvals", "pending")
}

// LivePath is the file a server attached to a companion keeps fresh.
func LivePath(agnosticHome string) string {
	return filepath.Join(agnosticHome, "local", "approvals", "companion.live")
}

// CompanionLive reports whether a companion touched path in the last
// LiveFresh.
func CompanionLive(path string, now time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	age := now.Sub(info.ModTime())
	return age >= -LiveFresh && age < LiveFresh
}

// TouchLive writes path with the time now, so its age says a companion
// is attached.
func TouchLive(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create approvals folder: %w", err)
	}
	if err := atomicfile.Replace(path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

// NewPendingID is 128 random bits in hex.
func NewPendingID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("pending request id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// ValidPendingID reports whether id is one NewPendingID makes, so it
// names a file inside the pending folder and nothing else.
func ValidPendingID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && strings.ToLower(id) == id
}

// PendingDigest ties a decision to one request: the tool, the session,
// the call and the raw input as the hook read it.
func PendingDigest(tool, session, call string, rawInput []byte) string {
	h := sha256.New()
	for _, part := range [][]byte{[]byte(tool), []byte(session), []byte(call), rawInput} {
		_, _ = fmt.Fprintf(h, "%d:", len(part))
		_, _ = h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ShowableInput is the input of call the panel can show in full. ok is
// false for a call the companion does not answer, an input over
// MaxPendingInput, or one that may hold a credential.
func ShowableInput(call string, input map[string]any) (PendingInput, bool) {
	text := func(key string) string {
		s, _ := input[key].(string)
		return s
	}
	var in PendingInput
	switch call {
	case "Bash":
		in.Command = text("command")
		if strings.TrimSpace(in.Command) == "" {
			return PendingInput{}, false
		}
	case "Read":
		in.FilePath = text("file_path")
	case "Write":
		in.FilePath, in.New = text("file_path"), text("content")
	case "Edit":
		in.FilePath, in.Old, in.New = text("file_path"), text("old_string"), text("new_string")
	case "MultiEdit":
		in.FilePath = text("file_path")
		edits, _ := input["edits"].([]any)
		var olds, news []string
		for _, e := range edits {
			edit, ok := e.(map[string]any)
			if !ok {
				return PendingInput{}, false
			}
			old, _ := edit["old_string"].(string)
			replacement, _ := edit["new_string"].(string)
			olds, news = append(olds, old), append(news, replacement)
		}
		if len(olds) == 0 {
			return PendingInput{}, false
		}
		in.Old, in.New = strings.Join(olds, "\n…\n"), strings.Join(news, "\n…\n")
	case "NotebookEdit":
		in.FilePath, in.New = text("notebook_path"), text("new_source")
	default:
		return PendingInput{}, false
	}
	if in.Command == "" && in.FilePath == "" {
		return PendingInput{}, false
	}
	fields := []string{in.Command, in.FilePath, in.Old, in.New}
	size := 0
	for _, f := range fields {
		size += len(f)
	}
	if size > MaxPendingInput {
		return PendingInput{}, false
	}
	if slices.ContainsFunc(fields, func(f string) bool { return f != "" && preview.ProseSensitive(f) }) {
		return PendingInput{}, false
	}
	return in, true
}

func pendingPath(dir, id string) string  { return filepath.Join(dir, id+".json") }
func decisionPath(dir, id string) string { return filepath.Join(dir, id+".decision.json") }

// WritePending saves p in dir, which only the user may read.
func WritePending(dir string, p Pending) error {
	if !ValidPendingID(p.ID) {
		return fmt.Errorf("pending request id %q is not valid", p.ID)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create pending folder: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect pending folder: %w", err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode pending request: %w", err)
	}
	return atomicfile.Replace(pendingPath(dir, p.ID), data, 0o600)
}

// ReadPending reads the request id in dir. ok is false when there is
// none.
func ReadPending(dir, id string) (Pending, bool, error) {
	if !ValidPendingID(id) {
		return Pending{}, false, nil
	}
	data, err := os.ReadFile(pendingPath(dir, id))
	if errors.Is(err, fs.ErrNotExist) {
		return Pending{}, false, nil
	}
	if err != nil {
		return Pending{}, false, fmt.Errorf("read pending request: %w", err)
	}
	var p Pending
	if err := json.Unmarshal(data, &p); err != nil || p.ID != id {
		return Pending{}, false, nil
	}
	return p, true, nil
}

// ListPending returns the requests in dir that wait for a decision,
// oldest first. A missing folder lists none; a file that does not
// parse is skipped.
func ListPending(dir string) ([]Pending, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending folder: %w", err)
	}
	var out []Pending
	for _, e := range entries {
		id, isRecord := strings.CutSuffix(e.Name(), ".json")
		if !isRecord || !ValidPendingID(id) {
			continue
		}
		if _, err := os.Lstat(decisionPath(dir, id)); err == nil {
			continue
		}
		p, ok, err := ReadPending(dir, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Pending) int {
		if c := a.Created.Compare(b.Created); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// WriteDecision saves the answer to request id.
func WriteDecision(dir, id string, d PendingDecision) error {
	if !ValidPendingID(id) || !slices.Contains(Decisions, d.Decision) {
		return fmt.Errorf("decision %q for %q is not valid", d.Decision, id)
	}
	data, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("encode decision: %w", err)
	}
	return atomicfile.Replace(decisionPath(dir, id), data, 0o600)
}

// ReadDecision reads the answer to request id. ok is false while none
// was written, or when the file does not parse.
func ReadDecision(dir, id string) (PendingDecision, bool) {
	data, err := os.ReadFile(decisionPath(dir, id))
	if err != nil {
		return PendingDecision{}, false
	}
	var d PendingDecision
	if json.Unmarshal(data, &d) != nil || !slices.Contains(Decisions, d.Decision) {
		return PendingDecision{}, false
	}
	return d, true
}

// RemovePending deletes request id and its decision.
func RemovePending(dir, id string) {
	if !ValidPendingID(id) {
		return
	}
	_ = os.Remove(pendingPath(dir, id))
	_ = os.Remove(decisionPath(dir, id))
}
