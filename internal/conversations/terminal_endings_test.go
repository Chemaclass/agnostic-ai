package conversations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// psTable puts a ps on PATH that answers `ps -o pid=,lstart= -p <pids>`
// and `ps -o lstart= -p <pid>` from the returned map of pid to start
// time, as the system ps does: one line per running pid, and exit 1
// with nothing on stderr when one of them does not run. Every other
// call lists nothing. Tests change the map and call sync to rewrite it.
func psTable(t *testing.T) (table map[int]string, sync func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("ps is not read on Windows")
	}
	bin := t.TempDir()
	file := filepath.Join(bin, "table")
	script := `#!/bin/sh
case "$*" in
*"lstart= -p "*) ;;
*) exit 0 ;;
esac
want=$(echo "$*" | sed 's/.*-p //' | tr ',' ' ')
status=0
for pid in $want; do
  line=$(grep "^$pid " "` + file + `")
  if [ -z "$line" ]; then status=1; continue; fi
  case "$*" in
  *"pid=,lstart="*) echo "$line" ;;
  *) echo "${line#* }" ;;
  esac
done
exit $status
`
	write(t, filepath.Join(bin, "ps"), script)
	if err := os.Chmod(filepath.Join(bin, "ps"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	table = map[int]string{}
	sync = func() {
		var lines []string
		for pid, start := range table {
			lines = append(lines, fmt.Sprintf("%d %s", pid, start))
		}
		write(t, file, strings.Join(lines, "\n")+"\n")
	}
	sync()
	return table, sync
}

const startedAt = "Sat Nov  7 09:00:00 2026"

func writeHeldSession(t *testing.T, stores Stores, pid int, id string) {
	t.Helper()
	write(t, filepath.Join(stores.ClaudeDir, "sessions", fmt.Sprintf("%d.json", pid)), fmt.Sprintf(`{"pid":%d,"sessionId":%q,"procStart":%q,"status":"idle"}`, pid, id, startedAt))
}

// endingScanner lists one Claude Code session with a reply the person
// has not seen, held by pid 4242, read through the system process table.
func endingScanner(t *testing.T) (scanner *Scanner, stores Stores, table map[int]string, sync func(), scan func() Conversation) {
	t.Helper()
	table, sync = psTable(t)
	scanner, stores = testScanner(t, noSQLite, SystemProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	if err := writeMarks(scanner.MarksPath, marksFile{SchemaVersion: marksSchemaVersion, Since: "2026-11-07T10:00:00Z", Marks: map[string]markEntry{}}); err != nil {
		t.Fatal(err)
	}
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00.000Z")
	writeHeldSession(t, stores, 4242, idSafari)
	table[4242] = startedAt
	sync()
	scan = func() Conversation {
		t.Helper()
		now = now.Add(freshSpan)
		return byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	}
	if c := scan(); !c.Live || !c.liveRead || c.State != StateUnread {
		t.Fatalf("the held session needs the person before it ends: %s", asJSON(t, c))
	}
	return scanner, stores, table, sync, scan
}

func TestScan_ATerminalSessionIsArchivedHoweverItsProcessEnds(t *testing.T) {
	for name, end := range map[string]func(stores Stores, table map[int]string){
		// Claude Code removes its session file when it exits.
		"exit or Ctrl+C": func(stores Stores, table map[int]string) {
			delete(table, 4242)
			_ = os.Remove(filepath.Join(stores.ClaudeDir, "sessions", "4242.json"))
		},
		// The process is gone and its session file stays behind.
		"closed tab, kill -9, crash or logout": func(_ Stores, table map[int]string) {
			delete(table, 4242)
		},
		// After a restart another process can take the same pid.
		"reboot": func(_ Stores, table map[int]string) {
			table[4242] = "Sun Nov  8 08:00:00 2026"
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, stores, table, sync, scan := endingScanner(t)
			end(stores, table)
			sync()
			if c := scan(); c.Live || !c.liveRead || c.State != StateDormant || !c.Unread {
				t.Errorf("an ended session is archived with its unread marker: %s", asJSON(t, c))
			}
		})
	}
}

func TestScan_ATerminalSessionStaysOpenWhileAProcessHoldsIt(t *testing.T) {
	// A detached tmux, screen or zellij session, a process stopped with
	// Ctrl+Z, and one that outlived its closed tab all still run, so ps
	// lists them. No terminal tab shows them; see TestOpen_FocusesTheExactTabOrSaysWhy.
	_, _, _, _, scan := endingScanner(t)
	if c := scan(); !c.Live || c.State != StateUnread || c.Open.Kind != RouteFocus || c.Open.Command == "" {
		t.Errorf("a held session stays open and keeps its resume command: %s", asJSON(t, c))
	}
}

func TestScan_ATerminalSessionIsArchivedOnlyOnceItsLastHolderEnds(t *testing.T) {
	_, stores, table, sync, scan := endingScanner(t)
	writeHeldSession(t, stores, 5151, idSafari)
	table[5151] = startedAt
	sync()
	delete(table, 4242)
	sync()
	if c := scan(); !c.Live || c.State == StateDormant {
		t.Errorf("a second holder keeps it open: %s", asJSON(t, c))
	}
	delete(table, 5151)
	sync()
	if c := scan(); c.Live || c.State != StateDormant {
		t.Errorf("it is archived once the last holder ends: %s", asJSON(t, c))
	}
}

func TestScan_AnOldClaudeSessionStaysOpenWhileHeld(t *testing.T) {
	scanner, _, table, sync, scan := endingScanner(t)
	later := time.Date(2026, 12, 20, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return later }
	if c := scan(); !c.Live || c.State == StateDormant {
		t.Errorf("a session held for weeks stays open: %s", asJSON(t, c))
	}
	delete(table, 4242)
	sync()
	scanner.Now = func() time.Time { return later.Add(freshSpan) }
	if c := scan(); c.Live || c.State != StateDormant {
		t.Errorf("it is archived once its holder ends: %s", asJSON(t, c))
	}
}

func TestScan_AnUnreadablePsCannotProveATerminalSessionEnded(t *testing.T) {
	_, _, _, _, scan := endingScanner(t)
	t.Setenv("PATH", t.TempDir())
	if c := scan(); c.liveRead || c.State != StateUnread || c.Unread {
		t.Errorf("without ps the session keeps the fallback and stays unread: %s", asJSON(t, c))
	}
}
