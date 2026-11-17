package conversations

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScan_AnOldCodexThreadStaysLiveUntilItsHolderLeaves(t *testing.T) {
	held := map[string]int{idSafari + ".lock": 4242}
	scanner := codexUnreadScanner(t, `[{"id":"`+idSafari+`","cwd":"/tmp/app","updated_at_ms":1762510000000,"originator":"codex-tui","first_user_message":"Keep this open"}]`, "", "")
	scanner.Processes = terminalProcesses(4242, held, nil)
	now := scanner.Now().Add(30 * 24 * time.Hour)
	scanner.Now = func() time.Time { return now }
	write(t, filepath.Join(scanner.Stores.CodexDir, "thread-writer-locks", idSafari+".lock"), "")
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if !c.Live || c.Open.Kind != RouteFocus || !c.liveRead {
		t.Errorf("an old held thread stays live and focuses its holder: %+v", c)
	}
	delete(held, idSafari+".lock")
	now = now.Add(freshSpan)
	c = byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if c.Live || !c.liveRead {
		t.Errorf("an unchanged thread is checked again within the refresh bound: %+v", c)
	}
}

func TestScan_AnOldCursorAgentChatStaysLiveUntilItsHolderLeaves(t *testing.T) {
	held := map[string]int{}
	scanner, store := cursorAgentScanner(t, terminalProcesses(4242, held, nil), cursorChat)
	held[store(cursorChat)] = 4242
	now := time.Now().Add(30 * 24 * time.Hour)
	scanner.Now = func() time.Time { return now }
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), cursorChat)
	if !c.Live || c.Open.Kind != RouteFocus || !c.liveRead {
		t.Errorf("an old held chat stays live and focuses its holder: %+v", c)
	}
	delete(held, store(cursorChat))
	now = now.Add(freshSpan)
	c = byID(t, scanner.Scan(context.Background(), ScanOptions{}), cursorChat)
	if c.Live || !c.liveRead {
		t.Errorf("an unchanged chat is checked again within the refresh bound: %+v", c)
	}
}

func TestScan_AnOldHeldCodexRunStaysListedUnderItsParent(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	scanner := codexRunScanner(t, stores, marksPath, runningSeen())
	now := scanner.Now().Add(30 * 24 * time.Hour)
	scanner.Now = func() time.Time { return now }
	view := scanner.Scan(context.Background(), ScanOptions{})
	if helpers := byID(t, view, idSafari).Helpers; len(helpers) != 1 || helpers[0].ID != runSeen {
		t.Errorf("an old run whose lock is held stays attached to its parent: %+v", helpers)
	}
}

func TestScan_AFailedCodexLockStatCannotProveClosure(t *testing.T) {
	scanner := codexUnreadScanner(t, `[{"id":"`+idSafari+`","cwd":"/tmp/app","updated_at_ms":1794049200000,"first_user_message":"Keep this open"}]`, "", "")
	lock := filepath.Join(scanner.Stores.CodexDir, "thread-writer-locks", idSafari+".lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(lock), lock); err != nil {
		t.Skip("cannot create a symlink:", err)
	}
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if c.liveRead {
		t.Error("a lock whose stat fails is unknown, not known closed")
	}
}

func TestScan_AnUnreadableClaudeSessionFileOfARunningProcessCannotProveClosure(t *testing.T) {
	for _, kind := range []string{"malformed", "invalid", "unreadable", "unlistable"} {
		for _, runs := range []bool{true, false} {
			t.Run(kind+" "+strconv.FormatBool(runs), func(t *testing.T) {
				alive := map[int]bool{4242: true, 5151: runs}
				scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: alive})
				writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
				writeMarkedSession(t, stores, idCache, "2026-11-07T11:00:00Z")
				sessions := filepath.Join(stores.ClaudeDir, "sessions")
				if kind == "unlistable" {
					write(t, sessions, "not a directory")
				} else {
					writeClaudeProcess(t, stores, 4242, idCache, "idle")
					file := filepath.Join(sessions, "5151.json")
					if kind == "malformed" {
						write(t, file, "{")
					} else if kind == "invalid" {
						write(t, file, "{}")
					} else if err := os.Mkdir(file, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				view := scanner.Scan(context.Background(), ScanOptions{})
				blocked := runs || kind == "unlistable"
				c := byID(t, view, idSafari)
				if blocked && (c.liveRead || c.State != StateActive) {
					t.Errorf("unreadable evidence of a running process keeps the existing fallback: %+v", c)
				}
				if !blocked && (!c.liveRead || c.State != StateDormant) {
					t.Errorf("a file a crash left behind, whose process is gone, does not block closure: %+v", c)
				}
				if kind != "unlistable" && !byID(t, view, idCache).Live {
					t.Error("a valid live entry survives an incomplete read")
				}
				note := claudeNote(view)
				if kind != "unlistable" && runs != strings.Contains(note, "5151.json") {
					t.Errorf("the Claude Code note names the blocking file only while its process runs: %q", note)
				}
			})
		}
	}
}

func claudeNote(view View) string {
	for _, source := range view.Tools {
		if source.Tool == ToolClaude {
			return source.Note
		}
	}
	return ""
}

func TestStillClosed_RefusesOnlyAnUnreadableClaudeSessionFileWhoseProcessRuns(t *testing.T) {
	for _, runs := range []bool{true, false} {
		t.Run(strconv.FormatBool(runs), func(t *testing.T) {
			var table []ProcessInfo
			if runs {
				table = []ProcessInfo{{PID: 5151, Started: "Sat Nov  7 09:00:00 2026"}}
			}
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{table: table})
			writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
			file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")
			measured, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(stores.ClaudeDir, "sessions", "5151.json")
			if err := os.MkdirAll(bad, 0o700); err != nil {
				t.Fatal(err)
			}
			refused := scanner.stillClosed(Conversation{ID: idSafari, file: file}, measured, "changed")
			switch {
			case runs && (refused == nil || refused.Kind != DeleteFailed || refused.Cause == nil || !strings.Contains(refused.Cause.Error(), bad)):
				t.Errorf("unreadable holder evidence of a running process refuses deletion and names its path: %v", refused)
			case !runs && refused != nil:
				t.Errorf("a file whose process is gone does not refuse deletion: %v", refused)
			}
		})
	}
}

func TestSystemProcesses_HoldersBoundsRequestsAndKeepsPartialEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lsof is not available on Windows")
	}
	bin := t.TempDir()
	command := filepath.Join(bin, "lsof")
	write(t, command, "#!/bin/sh\nshift 4\nif [ \"$#\" -gt 128 ]; then exit 2; fi\nfor file do\ncase \"$file\" in\n*fail.lock) echo 'cannot read files' >&2; exit 1;;\nesac\nprintf 'p4242\\ncnode\\nn%s\\n' \"$file\"\ndone\n")
	if err := os.Chmod(command, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var paths []string
	for i := range 300 {
		paths = append(paths, filepath.Join(bin, strconv.Itoa(i)+strings.Repeat("x", 200)+".lock"))
	}
	held, err := (SystemProcesses{}).Holders(paths)
	if err != nil || len(held) != len(paths) {
		t.Errorf("bounded requests read every path: got %d of %d, error %v", len(held), len(paths), err)
	}
	paths = append(paths, filepath.Join(bin, "fail.lock"))
	held, err = (SystemProcesses{}).Holders(paths)
	if err == nil || held[paths[0]] != 4242 {
		t.Errorf("a later failed request leaves absence unknown and keeps positive evidence: got %d holders, error %v", len(held), err)
	}
}
