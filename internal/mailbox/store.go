package mailbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const maxAudit = 500

type Store struct {
	Dir string
}

func (s Store) Send(ctx context.Context, from conversations.Ref, req SendRequest) (Receipt, error) {
	refused := ValidateSend(from, req)
	if refused == nil && req.To.Tool == "" {
		refused = NewRefusal("unknown-session")
	}
	if refused != nil {
		return Receipt{}, errors.Join(refused, s.RecordRefusal(ctx, from, req, refused))
	}

	release, err := s.lock(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer release()

	at := time.Now().UTC()
	thread, err := s.threadForSend(at, from, req)
	if err != nil {
		if refused, ok := errors.AsType[*Refusal](err); ok {
			err = errors.Join(refused, s.recordAudit(at, from, req, refused))
		}
		return Receipt{}, err
	}

	message := Message{
		N: len(thread.Messages) + 1, Type: req.Type, From: Actor{Ref: from, By: "human"},
		To: req.To, Body: conversations.CleanSendText(req.Body), State: Waiting, At: at, QueuedAt: at,
	}

	refused = thread.checkSend(from, req)
	if refused == nil {
		_, err = renderBlock(thread.ID, message)
		if err != nil {
			var ok bool
			refused, ok = errors.AsType[*Refusal](err)
			if !ok {
				return Receipt{}, err
			}
		}
	}
	if refused != nil {
		return Receipt{}, errors.Join(refused, s.recordRefusal(at, from, req, refused, thread))
	}

	thread.Messages = append(thread.Messages, message)
	action := "send"
	if message.Type == Done {
		thread.Status = Closing
		action = "close"
	}
	thread.addEvent(at, action, message.N)
	if err := s.writeThread(thread); err != nil {
		return Receipt{}, err
	}
	return Receipt{Thread: thread.ID, N: message.N, State: message.State}, nil
}

func (s Store) threadForSend(at time.Time, from conversations.Ref, req SendRequest) (Thread, error) {
	if req.Thread != "" {
		return s.loadThread(req.Thread)
	}
	id, err := s.newID(at)
	if err != nil {
		return Thread{}, err
	}
	return Thread{
		SchemaVersion: schemaVersion, ID: id, Status: Open, Cap: threadCap,
		Between: [2]conversations.Ref{from, req.To},
	}, nil
}

func (s Store) newID(at time.Time) (string, error) {
	for range 10 {
		var suffix [4]byte
		_, _ = rand.Read(suffix[:])
		id := "th_" + at.Format("20060102") + "_" + hex.EncodeToString(suffix[:])
		_, err := os.Lstat(s.threadPath(id))
		if errors.Is(err, fs.ErrNotExist) {
			return id, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("%s: could not allocate a thread ID", s.threadsDir())
}

func (s Store) RecordRefusal(ctx context.Context, from conversations.Ref, req SendRequest, refused *Refusal) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	at := time.Now().UTC()
	if threadID.MatchString(req.Thread) {
		thread, err := s.loadThread(req.Thread)
		if err == nil {
			return s.recordRefusal(at, from, req, refused, thread)
		}
	}
	return s.recordAudit(at, from, req, refused)
}

func (s Store) recordRefusal(at time.Time, from conversations.Ref, req SendRequest, refused *Refusal, thread Thread) error {
	if len(thread.Messages) == 0 {
		return s.recordAudit(at, from, req, refused)
	}
	stable := NewRefusal(refused.Guard)
	if stable.Guard == "cap" {
		stable = capRefusal(thread.Cap)
	}
	record := RefusalRecord{At: at, Type: knownType(req.Type), Guard: stable.Guard, Reason: stable.Reason}
	if i := slices.Index(thread.Between[:], from); i >= 0 {
		record.From = &thread.Between[i]
	}
	thread.Refusals = append(thread.Refusals, record)
	return s.writeThread(thread)
}

type auditRecord struct {
	At    time.Time          `json:"at"`
	Guard string             `json:"guard"`
	Type  string             `json:"type,omitempty"`
	From  conversations.Tool `json:"from,omitempty"`
	To    conversations.Tool `json:"to,omitempty"`
}

func (s Store) recordAudit(at time.Time, from conversations.Ref, req SendRequest, refused *Refusal) error {
	path := filepath.Join(s.Dir, "audit.json")
	var audit []auditRecord
	if err := readJSON(path, &audit); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	audit = append(audit, auditRecord{
		At: at, Guard: NewRefusal(refused.Guard).Guard, Type: knownType(req.Type),
		From: knownTool(from.Tool), To: knownTool(req.To.Tool),
	})
	return writeJSON(path, audit[max(0, len(audit)-maxAudit):])
}

func (s Store) Threads(ctx context.Context) (ThreadList, error) {
	return s.list(ctx, func(Thread) bool { return true })
}

func (s Store) ThreadsOf(ctx context.Context, ref conversations.Ref) (ThreadList, error) {
	return s.list(ctx, func(t Thread) bool { return t.includes(ref) })
}

func (s Store) list(ctx context.Context, keep func(Thread) bool) (ThreadList, error) {
	if err := ctx.Err(); err != nil {
		return ThreadList{}, err
	}
	list := ThreadList{Threads: []Thread{}}
	entries, err := s.threadEntries()
	if err != nil {
		return ThreadList{}, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return ThreadList{}, err
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasSuffix(entry.Name(), ".json") || !threadID.MatchString(id) {
			continue
		}
		thread, err := s.readThread(id)
		if err != nil {
			list.Warning = "A mailbox thread could not be read. The other threads are still available."
			continue
		}
		if keep(thread) {
			list.Threads = append(list.Threads, thread)
		}
	}
	slices.SortStableFunc(list.Threads, func(a, b Thread) int {
		return b.Messages[len(b.Messages)-1].At.Compare(a.Messages[len(a.Messages)-1].At)
	})
	return list, nil
}

func (s Store) Stamp() string {
	entries, err := s.threadEntries()
	if err != nil {
		return "unavailable"
	}
	if len(entries) == 0 {
		return ""
	}
	h := fnv.New64a()
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil {
			_, _ = fmt.Fprintf(h, "%s %d %d\n", entry.Name(), info.ModTime().UnixNano(), info.Size())
		} else {
			_, _ = fmt.Fprintln(h, entry.Name())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s Store) threadEntries() ([]fs.DirEntry, error) {
	if err := s.checkThreadDir(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return os.ReadDir(s.threadsDir())
}

func (s Store) checkThreadDir() error {
	if err := checkDir(s.Dir); err != nil {
		return err
	}
	return checkDir(s.threadsDir())
}

func (s Store) threadsDir() string { return filepath.Join(s.Dir, "threads") }

func (s Store) threadPath(id string) string {
	return filepath.Join(s.threadsDir(), id+".json")
}

func (s Store) loadThread(id string) (Thread, error) {
	if !threadID.MatchString(id) {
		return Thread{}, NewRefusal("format")
	}
	if err := s.checkThreadDir(); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Thread{}, missingThread()
		}
		return Thread{}, err
	}
	thread, err := s.readThread(id)
	if errors.Is(err, fs.ErrNotExist) {
		return Thread{}, missingThread()
	}
	return thread, err
}

func missingThread() *Refusal {
	return &Refusal{"not-in-thread", "This mailbox thread could not be found.", "Refresh the thread list and choose an available thread."}
}

func (s Store) readThread(id string) (Thread, error) {
	path := s.threadPath(id)
	var thread Thread
	if err := readJSON(path, &thread); err != nil {
		return Thread{}, err
	}
	if !thread.valid(id) {
		return Thread{}, fmt.Errorf("parse %s: invalid mailbox thread", path)
	}
	return thread, nil
}

func (s Store) writeThread(thread Thread) error {
	if err := privateDir(s.threadsDir()); err != nil {
		return err
	}
	return writeJSON(s.threadPath(thread.ID), thread)
}

func readJSON(path string, dst any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: expected a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("extra JSON value")
		}
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return atomicfile.Replace(path, append(data, '\n'), 0o600)
}
