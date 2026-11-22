package mailbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

var (
	claude = conversations.Ref{Tool: conversations.ToolClaude, ID: "4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e"}
	codex  = conversations.Ref{Tool: conversations.ToolCodex, ID: "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50"}
)

func sendQuestion(t *testing.T, s Store, thread string) Receipt {
	t.Helper()
	receipt, err := s.Send(context.Background(), claude, SendRequest{To: codex, Type: Question, Body: "How does this work?", Thread: thread})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func onlyThread(t *testing.T, s Store) Thread {
	t.Helper()
	list, err := s.Threads(context.Background())
	if err != nil || len(list.Threads) != 1 {
		t.Fatalf("threads = %v, %v", list, err)
	}
	return list.Threads[0]
}

func requireGuard(t *testing.T, err error, guard string) {
	t.Helper()
	var refused *Refusal
	if !errors.As(err, &refused) || refused.Guard != guard {
		t.Errorf("error = %v, want guard %q", err, guard)
	} else if refused.Reason == "" || refused.Next == "" {
		t.Errorf("refusal is missing guidance: %+v", refused)
	}
}

func TestStore_WritesPrivateMailboxAndFindsBothParticipants(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "mailbox")
	s := Store{Dir: root}
	before := s.Stamp()
	first := sendQuestion(t, s, "")
	if after := s.Stamp(); after == before {
		t.Error("creating a thread did not change the store stamp")
	}
	for _, who := range []conversations.Ref{claude, codex} {
		list, err := s.ThreadsOf(ctx, who)
		if err != nil || len(list.Threads) != 1 || list.Threads[0].ID != first.Thread {
			t.Errorf("threads for %v = %v, %v", who, list, err)
		}
	}
	thread := onlyThread(t, s)
	message := thread.Messages[0]
	if message.State != Waiting || message.QueuedAt.IsZero() || message.From.By != "human" || message.From.Ref != claude {
		t.Errorf("message = %+v", message)
	}
	if runtime.GOOS == "windows" {
		return
	}
	for path, want := range map[string]os.FileMode{
		root:                           0o700,
		filepath.Join(root, "threads"): 0o700,
		filepath.Join(root, ".lock"):   0o600,
		filepath.Join(root, "threads", first.Thread+".json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("stat %s: %v", path, err)
		} else if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestStore_KeepsBothConcurrentWritesToTheSameThread(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first := sendQuestion(t, s, "")
	var writers sync.WaitGroup
	for range 2 {
		writers.Go(func() {
			_, err := s.Send(context.Background(), codex, SendRequest{To: claude, Type: Finding, Body: "Use the lock.", Thread: first.Thread})
			if err != nil {
				t.Error(err)
			}
		})
	}
	writers.Wait()
	thread := onlyThread(t, s)
	if len(thread.Messages) != 3 {
		t.Fatalf("messages = %+v", thread.Messages)
	}
	for i, m := range thread.Messages {
		if m.N != i+1 {
			t.Errorf("message %d has number %d", i, m.N)
		}
	}
}

func TestStore_AuditsOnlyTheLast500RefusalsWithoutData(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir()}
	for i := range 501 {
		request := SendRequest{
			To: codex, Type: Finding, Body: fmt.Sprintf("api_key=example-value-%d", i),
		}
		if err := s.RecordRefusal(ctx, claude, request, NewRefusal("secret")); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir, "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "example-value") || strings.Contains(string(raw), claude.ID) || strings.Contains(string(raw), codex.ID) {
		t.Errorf("audit contains request data: %s", raw[:min(len(raw), 120)])
	}
	var audit []struct {
		Guard string `json:"guard"`
	}
	if err := json.Unmarshal(raw, &audit); err != nil || len(audit) != 500 {
		t.Errorf("audit length = %d, err = %v", len(audit), err)
	}
}

func TestStore_ReportsCorruptThreadAlongsideHealthyThreads(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	keep := sendQuestion(t, s, "")
	corrupt := sendQuestion(t, s, "")
	path := filepath.Join(s.Dir, "threads", corrupt.Thread+".json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := s.Threads(context.Background())
	if err != nil || len(list.Threads) != 1 || list.Threads[0].ID != keep.Thread || list.Warning == "" {
		t.Errorf("list = %+v, %v", list, err)
	}
}

func TestStore_RefusesToUseASymlinkAsTheLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink can require Windows privileges")
	}
	s := Store{Dir: t.TempDir()}
	other := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(other, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(s.Dir, ".lock")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Send(context.Background(), claude, SendRequest{To: codex, Type: Question, Body: "How does this work?"})
	if err == nil || !strings.Contains(err.Error(), "expected a regular lock file") {
		t.Errorf("send = %v, want lock file rejection", err)
	}
	data, err := os.ReadFile(other)
	if err != nil || string(data) != "keep me" {
		t.Errorf("unrelated file = %q, %v", data, err)
	}
}

func testResume(root string) *Resume {
	return &Resume{
		Worktree: root, Branch: "feature", Head: strings.Repeat("a", 40),
		Note: filepath.Join(root, "feature.md"), NoteKind: "manual",
	}
}

func TestStore_QueuesTaskHandoffWithoutChangingClaims(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(ctx, claude, Gives{root, []string{"scope/a.go"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	pointer := testResume(root)
	receipt, err := s.SendFromWorktree(ctx, claude, SendRequest{To: codex, Type: Handoff, Resume: pointer}, root)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != Waiting || receipt.Resume == nil || *receipt.Resume != *pointer {
		t.Errorf("handoff receipt = %+v", receipt)
	}
	message := onlyThread(t, s).Messages[0]
	if message.Type != Handoff || message.State != Waiting || message.Body != "" || message.Gives != nil ||
		message.Handoff != "" || message.Resume == nil || *message.Resume != *pointer {
		t.Errorf("stored handoff = %+v", message)
	}
	expectedOwner(t, s, root, "scope/a.go", claude)
}

func TestStore_RefusesInvalidHandoffPointers(t *testing.T) {
	root := t.TempDir()
	const secret = "api_key=example-secret-sentinel"
	for _, scenario := range []struct {
		name  string
		edit  func(*Resume)
		guard string
	}{
		{"relative worktree", func(r *Resume) { r.Worktree = "relative" }, "format"},
		{"short commit", func(r *Resume) { r.Head = "1234" }, "format"},
		{"relative note", func(r *Resume) { r.Note = "feature.md" }, "format"},
		{"missing note kind", func(r *Resume) { r.NoteKind = "" }, "format"},
		{"credential in worktree", func(r *Resume) { r.Worktree = filepath.Join(root, secret) }, "secret"},
		{"credential in branch", func(r *Resume) { r.Branch = secret }, "secret"},
		{"credential in note", func(r *Resume) { r.Note = filepath.Join(root, secret) }, "secret"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			pointer := testResume(root)
			scenario.edit(pointer)
			_, err := s.Send(t.Context(), claude, SendRequest{To: codex, Type: Handoff, Resume: pointer})
			requireGuard(t, err, scenario.guard)
			if list, err := s.Threads(t.Context()); err != nil || len(list.Threads) != 0 || list.Warning != "" {
				t.Errorf("threads after refusal = %+v, %v", list, err)
			}
		})
	}
}
