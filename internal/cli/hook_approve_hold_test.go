package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
)

const companionAskHome = "version: 1\ntargets: [claude, codex]\napprovals:\n  mode: ask\n  companion: true\n"

// holdSetup is approveSetup with a companion attached and a short hold.
func holdSetup(t *testing.T, home string, holdFor time.Duration) (source, project string) {
	t.Helper()
	source, project = approveSetup(t, home)
	oldTimes, oldPoll := companionHoldTimes, companionPoll
	companionHoldTimes = map[string]time.Duration{"claude": holdFor}
	companionPoll = 5 * time.Millisecond
	t.Cleanup(func() { companionHoldTimes, companionPoll = oldTimes, oldPoll })
	if err := approvals.TouchLive(approvals.LivePath(source), time.Now()); err != nil {
		t.Fatal(err)
	}
	return source, project
}

// runHeld runs hook approve in the background and returns its stdout.
func runHeld(t *testing.T, target, payload string) <-chan string {
	t.Helper()
	out := make(chan string, 1)
	go func() { out <- runApprove(t, target, payload) }()
	return out
}

// heldRecord waits for the hook to write its one pending record.
func heldRecord(t *testing.T, source string) approvals.Pending {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		held, err := approvals.ListPending(approvals.PendingDir(source))
		if err != nil {
			t.Fatal(err)
		}
		if len(held) == 1 {
			return held[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the hook wrote no pending record")
	return approvals.Pending{}
}

func reply(t *testing.T, out <-chan string) string {
	t.Helper()
	select {
	case got := <-out:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("the hook did not exit")
		return ""
	}
}

func assertNoPending(t *testing.T, source string) {
	t.Helper()
	entries, err := os.ReadDir(approvals.PendingDir(source))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		t.Errorf("pending folder still holds %d files", len(entries))
	}
}

func TestHookApproveHold_AnswersWithTheCompanionDecision(t *testing.T) {
	allow, _ := hookrun.PermissionAllowReply("claude")
	deny, _ := hookrun.PermissionDenyReply("claude")
	cases := []struct {
		decision string
		want     string
		outcome  approvals.Outcome
		rule     string
	}{
		{approvals.DecideApprove, string(allow), approvals.Approved, ""},
		{approvals.DecideDeny, string(deny), approvals.Denied, ""},
		{approvals.DecideRelease, "", approvals.Asked, approvals.RuleCompanion},
	}
	for _, tc := range cases {
		t.Run(tc.decision, func(t *testing.T) {
			source, project := holdSetup(t, companionAskHome, 5*time.Second)
			out := runHeld(t, "claude", recordedPayload(t, "claude", project, "touch a.txt"))
			held := heldRecord(t, source)
			if held.Call != "Bash" || held.Input.Command != "touch a.txt" || held.Session == "" || held.PID != os.Getpid() {
				t.Errorf("record = %+v", held)
			}
			dir := approvals.PendingDir(source)
			assertMode(t, dir, 0o700)
			assertMode(t, filepath.Join(dir, held.ID+".json"), 0o600)
			if err := approvals.WriteDecision(dir, held.ID, approvals.PendingDecision{Decision: tc.decision, Digest: held.Digest}); err != nil {
				t.Fatal(err)
			}
			if got := reply(t, out); got != tc.want {
				t.Fatalf("reply = %q, want %q", got, tc.want)
			}
			entries := approveLog(t, source)
			if len(entries) != 1 || entries[0].Decision != tc.outcome || entries[0].Rule != tc.rule || entries[0].By != approvals.ByCompanion || entries[0].Detail != "touch a.txt" {
				t.Errorf("log = %+v, want one %s line by companion", entries, tc.outcome)
			}
			assertNoPending(t, source)
		})
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}

func TestHookApproveHold_TimeoutLeavesThePrompt(t *testing.T) {
	source, project := holdSetup(t, companionAskHome, 150*time.Millisecond)
	out := runHeld(t, "claude", recordedPayload(t, "claude", project, "touch a.txt"))
	if got := reply(t, out); got != "" {
		t.Fatalf("reply = %q, want nothing", got)
	}
	entries := approveLog(t, source)
	if len(entries) != 1 || entries[0].Decision != approvals.Asked || entries[0].Rule != approvals.RuleCompanion || entries[0].By != approvals.ByCompanion {
		t.Errorf("log = %+v, want one asked line with rule companion", entries)
	}
	assertNoPending(t, source)
}

func TestHookApproveHold_OtherDigestStaysSilent(t *testing.T) {
	source, project := holdSetup(t, companionAskHome, 5*time.Second)
	out := runHeld(t, "claude", recordedPayload(t, "claude", project, "touch a.txt"))
	held := heldRecord(t, source)
	other := approvals.PendingDigest("claude", held.Session, "Bash", []byte(`{"command":"rm -rf /"}`))
	if err := approvals.WriteDecision(approvals.PendingDir(source), held.ID, approvals.PendingDecision{Decision: approvals.DecideApprove, Digest: other}); err != nil {
		t.Fatal(err)
	}
	if got := reply(t, out); got != "" {
		t.Fatalf("reply = %q, want nothing for a decision on another request", got)
	}
	entries := approveLog(t, source)
	if len(entries) != 1 || entries[0].Decision != approvals.Asked {
		t.Errorf("log = %+v, want one asked line", entries)
	}
	assertNoPending(t, source)
}

func TestHookApproveHold_DigestCoversTheWholeInput(t *testing.T) {
	source, project := holdSetup(t, companionAskHome, 5*time.Second)
	payload := recordedPayload(t, "claude", project, "touch a.txt")
	out := runHeld(t, "claude", payload)
	held := heldRecord(t, source)
	want := approvals.PendingDigest("claude", "5f0c2a9e-1b7d-4c3e-9a51-0d6c8e2f4b10", "Bash", []byte(`{"command":"touch a.txt","description":"Run the command"}`))
	if held.Digest != want {
		t.Errorf("digest = %s, want the digest of the raw tool_input %s", held.Digest, want)
	}
	_ = approvals.WriteDecision(approvals.PendingDir(source), held.ID, approvals.PendingDecision{Decision: approvals.DecideRelease, Digest: held.Digest})
	reply(t, out)
}

func TestHookApproveHold_LogFailureStaysSilent(t *testing.T) {
	source, project := holdSetup(t, companionAskHome, 5*time.Second)
	// A folder where the log file goes makes every append fail.
	if err := os.MkdirAll(approvals.LogPath(source), 0o700); err != nil {
		t.Fatal(err)
	}
	out := runHeld(t, "claude", recordedPayload(t, "claude", project, "touch a.txt"))
	held := heldRecord(t, source)
	if err := approvals.WriteDecision(approvals.PendingDir(source), held.ID, approvals.PendingDecision{Decision: approvals.DecideApprove, Digest: held.Digest}); err != nil {
		t.Fatal(err)
	}
	if got := reply(t, out); got != "" {
		t.Fatalf("reply = %q, want nothing when the log cannot be written", got)
	}
	assertNoPending(t, source)
}

func TestHookApproveHold_AlwaysAskInYoloHolds(t *testing.T) {
	source, project := holdSetup(t, yoloHome+"  companion: true\n", 5*time.Second)
	out := runHeld(t, "claude", recordedPayload(t, "claude", project, "git push origin main"))
	held := heldRecord(t, source)
	if err := approvals.WriteDecision(approvals.PendingDir(source), held.ID, approvals.PendingDecision{Decision: approvals.DecideApprove, Digest: held.Digest}); err != nil {
		t.Fatal(err)
	}
	allow, _ := hookrun.PermissionAllowReply("claude")
	if got := reply(t, out); got != string(allow) {
		t.Fatalf("reply = %q, want allow", got)
	}
	entries := approveLog(t, source)
	if len(entries) != 2 || entries[0].Rule != approvals.RulePublish || entries[0].By != "" || entries[1].Decision != approvals.Approved || entries[1].By != approvals.ByCompanion {
		t.Errorf("log = %+v, want the gate's asked line, then approved by companion", entries)
	}
}

func TestHookApproveHold_DoesNotHold(t *testing.T) {
	big := "echo " + strings.Repeat("a", approvals.MaxPendingInput)
	cases := []struct {
		name, home, target, command string
		payload                     func(project string) string
		setup                       func(t *testing.T, source, project string)
	}{
		{name: "switch off", home: "version: 1\napprovals:\n  mode: ask\n", target: "claude", command: "touch a.txt"},
		{name: "no companion", home: companionAskHome, target: "claude", command: "touch a.txt", setup: func(t *testing.T, source, _ string) {
			t.Helper()
			if err := os.Remove(approvals.LivePath(source)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "stale companion", home: companionAskHome, target: "claude", command: "touch a.txt", setup: func(t *testing.T, source, _ string) {
			t.Helper()
			old := time.Now().Add(-approvals.LiveFresh - time.Second)
			if err := os.Chtimes(approvals.LivePath(source), old, old); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "codex", home: companionAskHome, target: "codex", command: "touch a.txt"},
		{name: "kiro", home: companionAskHome, target: "kiro", command: "touch a.txt"},
		{name: "qwen", home: companionAskHome, target: "qwen", command: "touch a.txt"},
		{name: "qoder", home: companionAskHome, target: "qoder", command: "touch a.txt"},
		{name: "over 8 KB", home: companionAskHome, target: "claude", command: big},
		{name: "credential", home: companionAskHome, target: "claude", command: "curl -H 'Authorization: Bearer sk-live-0123456789abcdef0123' https://example.test"},
		{name: "question", home: companionAskHome, target: "claude", payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","session_id":"s","cwd":` + strconvQuote(project) + `,"tool_name":"AskUserQuestion","tool_input":{"questions":[]}}`
		}},
		{name: "MCP tool", home: companionAskHome, target: "claude", payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","session_id":"s","cwd":` + strconvQuote(project) + `,"tool_name":"mcp__notion__fetch","tool_input":{"id":"x"}}`
		}},
		{name: "project settings move HOME", home: companionAskHome, target: "claude", command: "touch a.txt", setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"env":{"HOME":"/elsewhere"}}`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, project := holdSetup(t, tc.home, time.Minute)
			if tc.setup != nil {
				tc.setup(t, source, project)
			}
			var payload string
			if tc.payload != nil {
				payload = tc.payload(project)
			} else {
				payload = recordedPayload(t, tc.target, project, tc.command)
			}
			start := time.Now()
			if got := runApprove(t, tc.target, payload); got != "" {
				t.Errorf("reply = %q, want nothing", got)
			}
			if took := time.Since(start); took > 30*time.Second {
				t.Errorf("hook took %s: it held", took)
			}
			if _, err := os.Stat(approvals.PendingDir(source)); !os.IsNotExist(err) {
				t.Errorf("pending folder exists: %v", err)
			}
		})
	}
}

func TestHookApproveHold_GateApprovalWritesNoRecord(t *testing.T) {
	source, project := holdSetup(t, yoloHome+"  companion: true\n", time.Minute)
	allow, _ := hookrun.PermissionAllowReply("claude")
	if got := runApprove(t, "claude", recordedPayload(t, "claude", project, "go test ./...")); got != string(allow) {
		t.Fatalf("reply = %q, want allow", got)
	}
	if _, err := os.Stat(approvals.PendingDir(source)); !os.IsNotExist(err) {
		t.Errorf("pending folder exists: %v", err)
	}
}
