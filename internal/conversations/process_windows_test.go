//go:build windows

package conversations

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsProcesses_ListThisProcessWithItsStartAndParent(t *testing.T) {
	table, err := SystemProcesses{}.Table()
	if err != nil {
		t.Fatal(err)
	}
	for _, process := range table {
		if process.PID == os.Getpid() {
			if process.PPID != os.Getppid() || process.Started == "" || process.Command != strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
				t.Errorf("this process: %+v", process)
			}
			if !(SystemProcesses{}).Alive(os.Getpid(), process.Started) || (SystemProcesses{}).Alive(os.Getpid(), "1") {
				t.Error("Alive does not check the start")
			}
			return
		}
	}
	t.Errorf("pid %d is not among %d processes", os.Getpid(), len(table))
}

func TestWindowsProcesses_ReadTheCommandLine(t *testing.T) {
	line := SystemProcesses{}.Args([]int{os.Getpid()})[os.Getpid()]
	if !strings.Contains(line, filepath.Base(os.Args[0])) {
		t.Errorf("command line %q", line)
	}
}

func TestWindowsProcesses_NameTheProcessThatHoldsAFile(t *testing.T) {
	dir := t.TempDir()
	held, free := filepath.Join(dir, "locks", "held.lock"), filepath.Join(dir, "locks", "free.lock")
	if err := os.MkdirAll(filepath.Dir(held), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{held, free} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	holder := exec.Command("powershell.exe", "-NoProfile", "-Command", "$f = [IO.File]::Open($env:HELD, 'Open', 'ReadWrite', 'ReadWrite'); 'open'; Start-Sleep 60")
	holder.Env = append(os.Environ(), "HELD="+held)
	out, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill() }()
	ready := make([]byte, 4)
	if _, err := out.Read(ready); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	holders, err := SystemProcesses{}.Holders([]string{held, free})
	if err != nil || holders[held] != holder.Process.Pid || len(holders) != 1 {
		t.Errorf("holders %v err %v; want %s held by %d", holders, err, held, holder.Process.Pid)
	}
	under, err := SystemProcesses{}.HoldersUnder(dir)
	if err != nil || under[held] != holder.Process.Pid || len(under) != 1 {
		t.Errorf("under %v err %v", under, err)
	}
}
