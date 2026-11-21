package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// TestMailboxCommands_NeverReachALauncher runs every agent command with a
// launcher that records each call: none may type, paste, focus or start.
func TestMailboxCommands_NeverReachALauncher(t *testing.T) {
	spy := &recordingLauncher{}
	previous := systemLauncher
	systemLauncher = func() conversations.Launcher { return spy }
	t.Cleanup(func() { systemLauncher = previous })

	f := newMailboxAgentFixture(t)
	f.approval(false)
	root := claimFixtureRepo(t, f)
	f.as(agentClaude)
	id := threadOf(t, f.mustSend(question(agentCodex)))
	run := func(input string, args ...string) {
		t.Helper()
		if out, err := runRootWithInput(t, input, args...); err != nil {
			t.Fatalf("%v = %q, %v", args, out, err)
		}
	}
	run("", "mailbox", "claim", "src")
	run(fmt.Sprintf(`{"thread":%q,"type":"handoff","body":"Take the parser.","gives":{"worktree":%q,"paths":["src"]}}`, id, root), "mailbox", "send")
	run("", "mailbox", "list")
	run("", "mailbox", "show", id)
	f.as(agentCodex)
	run(`{"thread":"`+id+`","type":"finding","body":"Yes."}`, "mailbox", "send")
	run("", "mailbox", "accept", "--thread", id, "--n", "2")
	run("", "mailbox", "release", "src")
	f.as(agentClaude)
	second := threadOf(t, f.mustSend(question(agentCodex)))
	if _, err := runRoot(t, "mailbox", "claim", "src"); err != nil {
		t.Fatal(err)
	}
	offer := fmt.Sprintf(`{"thread":%q,"type":"handoff","body":"Take it back.","gives":{"worktree":%q,"paths":["src"]}}`, second, root)
	run(offer, "mailbox", "send")
	f.as(agentCodex)
	run("", "mailbox", "decline", "--thread", second, "--n", "2")
	run("", "mailbox", "done", "--thread", id)

	if len(spy.calls) != 0 {
		t.Errorf("mailbox commands called the launcher: %q", spy.calls)
	}
}

// TestMailboxCommands_ReachNoTerminalCode keeps the spy honest: the agent
// commands and the store must not name a launcher, the clipboard or a
// process start, which a spy on systemLauncher alone would miss.
func TestMailboxCommands_ReachNoTerminalCode(t *testing.T) {
	files, err := filepath.Glob("mailbox*.go")
	if err != nil {
		t.Fatal(err)
	}
	store, err := filepath.Glob(filepath.Join("..", "mailbox", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(files, store...) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		banned := []string{"Launcher", "SendComposed", "Clipboard", "TypeInTab", "FocusTab"}
		if filepath.Base(path) != "gitrefs.go" {
			banned = append(banned, `"os/exec"`)
		}
		for _, word := range banned {
			if strings.Contains(string(data), word) {
				t.Errorf("%s names %s", path, word)
			}
		}
	}
}
