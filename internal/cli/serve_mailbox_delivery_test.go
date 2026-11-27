package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

const nativeClaudeMailboxHook = `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"agnostic-ai project -- hook mailbox --target claude"}]}]}}`

type mailboxWaitingReply struct {
	From  conversations.Ref
	To    conversations.Ref
	N     int
	Type  string
	Setup string
}

func TestMailboxHTTP_WaitingSetupTracksAnOffPageReceiver(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", h.scanner.Stores.ClaudeDir)
	var target string
	for _, row := range listConversations(t, h).Conversations {
		if row.ID == inWorktree {
			target = row.Folder
		}
	}
	if target == "" {
		t.Fatal("fixture receiver has no folder")
	}
	writeFile(t, filepath.Join(target, "agnostic-ai.yaml"), "version: 1\nbuiltins: [memory]\n")
	mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	request := func(query, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+query, nil)
		req.Header.Set("If-None-Match", etag)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	check := func(rec *httptest.ResponseRecorder, want string) {
		t.Helper()
		var result struct {
			Conversations []struct {
				ID      string
				Threads []struct {
					Waiting        int
					WaitingMessage *mailboxWaitingReply
				}
			}
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("list = %d %s", rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Conversations) != 1 || result.Conversations[0].ID != inProject || len(result.Conversations[0].Threads) != 1 {
			t.Fatalf("the list did not hold only the sender and its thread: %#v", result.Conversations)
		}
		thread := result.Conversations[0].Threads[0]
		expected := mailboxWaitingReply{
			From: conversations.Ref{Tool: conversations.ToolClaude, ID: inProject},
			To:   conversations.Ref{Tool: conversations.ToolClaude, ID: inWorktree},
			N:    1, Type: "question", Setup: want,
		}
		if thread.Waiting != 1 || thread.WaitingMessage == nil || *thread.WaitingMessage != expected {
			t.Errorf("thread = %#v, waiting message = %+v, want %+v", thread, thread.WaitingMessage, expected)
		}
		if !strings.Contains(rec.Body.String(), `"recipientLive":false`) {
			t.Error("waiting message did not identify the stopped off-page receiver")
		}
	}

	const senderQuery = "?q=Fix+the+parser"
	first := request(senderQuery, "")
	check(first, "off")
	tag := first.Header().Get("ETag")
	if rec := request(senderQuery, tag); rec.Code != http.StatusNotModified {
		t.Errorf("unchanged delivery inputs = %d, want 304", rec.Code)
	}
	otherQuery := "?q=This+does+not+match+a+conversation"
	otherTag := request(otherQuery, "").Header().Get("ETag")
	if rec := request(senderQuery, tag); rec.Code != http.StatusNotModified {
		t.Errorf("sender after another query = %d, want 304", rec.Code)
	}
	if rec := request(otherQuery, otherTag); rec.Code != http.StatusNotModified {
		t.Errorf("other query after sender = %d, want 304", rec.Code)
	}
	writeFile(t, filepath.Join(target, "agnostic-ai.local.yaml"), "builtins: [mailbox]\n")
	writeFile(t, filepath.Join(target, ".claude", "settings.json"), nativeClaudeMailboxHook)
	next := request(senderQuery, tag)
	check(next, "auto")
	if next.Header().Get("ETag") == tag {
		t.Error("hook installation did not refresh the list tag")
	}
}

func TestMailboxSummary_UsesQueueOrderForTheWaitingMessage(t *testing.T) {
	from := conversations.Ref{Tool: conversations.ToolClaude, ID: inProject}
	to := conversations.Ref{Tool: conversations.ToolCodex, ID: inWorktree}
	now := time.Unix(10, 0)
	thread := mailbox.Thread{Messages: []mailbox.Message{
		{N: 1, Type: "question", From: mailbox.Actor{Ref: from}, To: to, State: mailbox.Waiting, QueuedAt: now.Add(time.Minute)},
		{N: 2, Type: "handoff", From: mailbox.Actor{Ref: to}, To: from, State: mailbox.Waiting, QueuedAt: now},
		{N: 3, Type: "finding", From: mailbox.Actor{Ref: from}, To: to, State: mailbox.Held},
	}}
	data, err := json.Marshal(mailboxSummary(thread, to))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Waiting        int
		Held           int
		WaitingMessage *mailboxWaitingReply
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	want := mailboxWaitingReply{From: to, To: from, N: 2, Type: "handoff", Setup: "unknown"}
	if result.Waiting != 2 || result.Held != 1 || result.WaitingMessage == nil || *result.WaitingMessage != want {
		t.Errorf("summary = %s, waiting message = %+v, want %+v", data, result.WaitingMessage, want)
	}
}

func TestMailboxDeliverySetup_UsesTheReceiversNativeHookState(t *testing.T) {
	for _, test := range []struct {
		name, tool, projectHook, userConfig, setup string
	}{
		{"claude with no installed hook", "claude", "", "", "off"},
		{"claude with malformed settings", "claude", "{bad", "", "unknown"},
		{"claude disabled", "claude", `{"disableAllHooks":true}`, "", "disabled"},
		{"codex needs trust", "codex", nativeCodexMailboxHook, "", "trust"},
		{"codex hook disabled", "codex", nativeCodexMailboxHook, "enabled = false", "disabled"},
		{"codex hook trusted", "codex", nativeCodexMailboxHook, codexMailboxTrustedHash, "auto"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, home := realDir(t), realDir(t)
			t.Setenv("AGNOSTIC_AI_HOME", home)
			t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
			t.Setenv("CODEX_HOME", home)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nbuiltins: [mailbox]\n")
			if test.projectHook != "" {
				file := filepath.Join(root, ".claude", "settings.json")
				if test.tool == "codex" {
					file = filepath.Join(root, ".codex", "hooks.json")
					settings := fmt.Sprintf("[projects.%q]\ntrust_level = \"trusted\"\n[hooks.state.%q]\n%s\n", root, file+":user_prompt_submit:0:0", test.userConfig)
					writeFile(t, filepath.Join(home, "config.toml"), settings)
				}
				writeFile(t, file, test.projectHook)
			}
			checks := newMailboxDeliveryChecks()
			row := conversations.Conversation{Tool: conversations.Tool(test.tool), Folder: root}
			if setup := checks.setup(row); setup != test.setup {
				t.Errorf("delivery setup = %q, want %q", setup, test.setup)
			}
		})
	}
}

func TestMailboxDeliverySetup_ConfinesProjectInputs(t *testing.T) {
	for _, mode := range []string{"unrelated parent", "symlinked settings", "symlinked settings directory", "oversized settings", "special settings"} {
		t.Run(mode, func(t *testing.T) {
			parent, home := realDir(t), realDir(t)
			t.Setenv("AGNOSTIC_AI_HOME", home)
			t.Setenv("CLAUDE_CONFIG_DIR", home)
			root := filepath.Join(parent, "receiver")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			const enabled = "version: 1\nbuiltins: [mailbox]\n"
			writeFile(t, filepath.Join(parent, "agnostic-ai.yaml"), enabled)
			outside := filepath.Join(parent, ".claude", "settings.json")
			writeFile(t, outside, nativeClaudeMailboxHook)
			want := "unknown"
			if mode == "unrelated parent" {
				want = "off"
			} else {
				writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), enabled)
				settings := filepath.Join(root, ".claude", "settings.json")
				if mode != "symlinked settings directory" {
					if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				switch mode {
				case "symlinked settings", "symlinked settings directory":
					if mode == "symlinked settings directory" {
						outside, settings = filepath.Dir(outside), filepath.Dir(settings)
					}
					if err := os.Symlink(outside, settings); err != nil {
						if runtime.GOOS == "windows" {
							t.Skipf("symlink unavailable: %v", err)
						}
						t.Fatal(err)
					}
				case "oversized settings":
					writeFile(t, settings, nativeClaudeMailboxHook+strings.Repeat(" ", mailboxDeliveryFileLimit))
				case "special settings":
					if runtime.GOOS == "windows" {
						t.Skip("POSIX filesystem FIFOs are unavailable on Windows")
					}
					mkfifo, err := exec.LookPath("mkfifo")
					if err != nil {
						t.Skip("mkfifo unavailable")
					}
					if data, err := exec.Command(mkfifo, settings).CombinedOutput(); err != nil {
						t.Fatalf("mkfifo: %v: %s", err, data)
					}
				}
			}
			checks := newMailboxDeliveryChecks()
			row := conversations.Conversation{Tool: conversations.ToolClaude, Folder: root}
			if setup := checks.setup(row); setup != want {
				t.Errorf("delivery setup = %q, want %q", setup, want)
			}
		})
	}
}

func TestMailboxDeliverySetup_AnchorsTheUserOverlay(t *testing.T) {
	root, home, other := realDir(t), realDir(t), realDir(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), nativeClaudeMailboxHook)
	writeFile(t, filepath.Join(other, "agnostic-ai.yaml"), "version: 1\nbuiltins: [mailbox]\n")
	link := filepath.Join(home, "local")
	if err := os.Symlink(other, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	row := conversations.Conversation{Tool: conversations.ToolClaude, Folder: root}
	checks := newMailboxDeliveryChecks()
	if setup := checks.setup(row); setup != "unknown" {
		t.Errorf("delivery setup from escaped user overlay = %q, want unknown", setup)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(link, "agnostic-ai.yaml"), "version: 1\nbuiltins: [mailbox]\n")
	checks = newMailboxDeliveryChecks()
	if setup := checks.setup(row); setup != "auto" {
		t.Errorf("delivery setup from in-root user overlay = %q, want auto", setup)
	}
}

func TestMailboxDeliverySetup_ReadsClaudeSharedSettingsFromTheReceiverFolder(t *testing.T) {
	root, home := realDir(t), realDir(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nbuiltins: [mailbox]\n")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), nativeClaudeMailboxHook)
	folder := filepath.Join(root, "nested")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	row := conversations.Conversation{Tool: conversations.ToolClaude, Folder: folder}
	checks := newMailboxDeliveryChecks()
	if setup := checks.setup(row); setup != "off" {
		t.Errorf("delivery setup with only checkout shared settings = %q, want off", setup)
	}
	writeFile(t, filepath.Join(folder, ".claude", "settings.json"), nativeClaudeMailboxHook)
	checks = newMailboxDeliveryChecks()
	if setup := checks.setup(row); setup != "auto" {
		t.Errorf("delivery setup with receiver shared settings = %q, want auto", setup)
	}
	writeFile(t, filepath.Join(root, ".claude", "settings.local.json"), `{"disableAllHooks":true}`)
	want := "disabled"
	if runtime.GOOS == "windows" {
		want = "auto"
	}
	checks = newMailboxDeliveryChecks()
	if setup := checks.setup(row); setup != want {
		t.Errorf("delivery setup with checkout local override = %q, want %s", setup, want)
	}
}

func TestMailboxDeliverySetup_DoesNotInferCodexHooksFromAnUnresolvedLayer(t *testing.T) {
	for _, input := range []string{".codex/config.toml", ".codex/hooks.json", ".git", "nested trust"} {
		t.Run(input, func(t *testing.T) {
			root, home := realDir(t), realDir(t)
			t.Setenv("AGNOSTIC_AI_HOME", home)
			t.Setenv("CODEX_HOME", home)
			if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\nbuiltins: [mailbox]\n")
			file := filepath.Join(root, ".codex", "hooks.json")
			writeFile(t, file, nativeCodexMailboxHook)
			settings := fmt.Sprintf("[projects.%q]\ntrust_level = \"trusted\"\n[hooks.state.%q]\n%s\n", root, file+":user_prompt_submit:0:0", codexMailboxTrustedHash)
			writeFile(t, filepath.Join(home, "config.toml"), settings)
			receiver := filepath.Join(root, "nested")
			if err := os.Mkdir(receiver, 0o700); err != nil {
				t.Fatal(err)
			}
			row := conversations.Conversation{Tool: conversations.ToolCodex, Folder: receiver}
			checks := newMailboxDeliveryChecks()
			if setup := checks.setup(row); setup != "auto" {
				t.Errorf("delivery setup from checkout = %q, want auto", setup)
			}
			var watch mailboxDeliveryWatch
			var query mailboxDeliveryQuery
			stamp := watch.remember(query, checks.inputs)
			switch input {
			case ".git":
				marker := filepath.Join(root, ".git")
				if err := os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				writeFile(t, marker, "gitdir: /not-read\n")
			case ".codex/config.toml":
				writeFile(t, filepath.Join(receiver, filepath.FromSlash(input)), "[features]\nhooks = false\n")
			case ".codex/hooks.json":
				writeFile(t, filepath.Join(receiver, filepath.FromSlash(input)), nativeCodexMailboxHook)
			case "nested trust":
				settings += fmt.Sprintf("[projects.%q]\ntrust_level = \"untrusted\"\n", receiver)
				writeFile(t, filepath.Join(home, "config.toml"), settings)
			}
			if next := watch.stamp(query); next == stamp {
				t.Error("creating a Codex config source did not change the stamp")
			}
			checks = newMailboxDeliveryChecks()
			if setup := checks.setup(row); setup != "unknown" {
				t.Errorf("delivery setup with unresolved %s = %q, want unknown", input, setup)
			}
		})
	}
}

func TestMailboxDeliverySetup_LeavesInlineCodexHooksUnknown(t *testing.T) {
	root, home := realDir(t), realDir(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	t.Setenv("CODEX_HOME", home)
	writeFile(t, filepath.Join(home, "config.toml"), "[[hooks.UserPromptSubmit]]\nmatcher = '*'\n")
	checks := newMailboxDeliveryChecks()
	row := conversations.Conversation{Tool: conversations.ToolCodex, Folder: root}
	if setup := checks.setup(row); setup != "unknown" {
		t.Errorf("delivery setup with inline hooks = %q, want unknown", setup)
	}
}

func TestMailboxPromptCommand_RecognizesOnlyMailboxInvocations(t *testing.T) {
	const direct = "agnostic-ai project -- hook mailbox --target codex"
	for _, test := range []struct {
		command string
		want    bool
	}{
		{direct, true},
		{"echo " + direct, false},
		{direct + "; exit 1", false},
		{"export AGNOSTIC_AI_TARGET=codex; " + direct, true},
	} {
		if got := mailboxPromptCommand(test.command, conversations.ToolCodex); got != test.want {
			t.Errorf("mailboxPromptCommand(%q) = %t, want %t", test.command, got, test.want)
		}
	}
	if !mailboxPromptCommand(mailboxPromptSpec().Command, conversations.ToolClaude) {
		t.Error("Claude built-in wrapper was not recognized")
	}
	codexCommand := mailboxPromptSpec().Codex.Command
	if runtime.GOOS == "windows" {
		codexCommand = mailboxPromptSpec().CommandWindows
	}
	if !mailboxPromptCommand(codexCommand, conversations.ToolCodex) {
		t.Error("Codex built-in wrapper was not recognized")
	}
}

const nativeCodexMailboxHook = `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"agnostic-ai project -- hook mailbox --target codex"}]}]}}`

const codexMailboxTrustedHash = `trusted_hash = "sha256:5c48bd333e40391208ad227bc71180bed0054c0ff156e5c305448993025885bb"`
