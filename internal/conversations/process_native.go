package conversations

import (
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// nativeReader reads processes through the system's own API, on a
// system with no ps or lsof: Windows.
type nativeReader interface {
	// processes lists every process. Started is its creation time as
	// filetimeText writes it; Command is its executable's name.
	processes() ([]ProcessInfo, error)
	args(pids []int) map[int]string
	holders(paths []string) (map[string]int, error)
}

// filetimeEpochTicks is 1601-01-01, where a Windows file time starts,
// in .NET ticks: 100 ns steps from year 1.
const filetimeEpochTicks = 504911232000000000

// filetimeText writes a process creation time, in 100 ns steps since
// 1601, to the microsecond: PowerShell reads creation times to the
// microsecond, so both forms compare equal.
func filetimeText(filetime uint64) string {
	return strconv.FormatUint(filetime/10*10, 10)
}

// windowsProcStart is the start a Windows Claude Code session file
// records, written as filetimeText. The native claude.exe writes a file
// time, in procStart or procStartFt. A Claude Code run by Node writes
// procStart as the .NET ticks of PowerShell's creation time in local
// time, which are larger than any file time of this era. Any other
// value is kept.
func windowsProcStart(procStart, procStartFt string, local *time.Location) string {
	if procStart == "" {
		procStart = procStartFt
	}
	ticks, err := strconv.ParseUint(strings.TrimSpace(procStart), 10, 64)
	switch {
	case err != nil || ticks < unixEpochFiletime:
		return procStart
	case ticks < filetimeEpochTicks:
		return filetimeText(ticks)
	}
	wall := time.Unix(0, 0).UTC().Add(time.Duration(ticks-filetimeEpochTicks-unixEpochFiletime) * 100)
	at := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), local)
	return filetimeText(uint64(at.UnixNano()/100) + unixEpochFiletime)
}

// unixEpochFiletime is 1970-01-01 as a file time.
const unixEpochFiletime = 116444736000000000

// holdersByHalves maps each of paths to a process that holds it. list names the
// processes that hold any of a group of paths, so a group is split in
// two only while someone holds one of it: few files are held at once.
func holdersByHalves(paths []string, list func(group []string) ([]int, error)) (map[string]int, error) {
	held := map[string]int{}
	var find func(group []string) error
	find = func(group []string) error {
		pids, err := list(group)
		if err != nil || len(pids) == 0 {
			return err
		}
		if len(group) == 1 {
			held[group[0]] = pids[0]
			return nil
		}
		if err := find(group[:len(group)/2]); err != nil {
			return err
		}
		return find(group[len(group)/2:])
	}
	for len(paths) > 0 {
		count := min(len(paths), 256)
		if err := find(paths[:count]); err != nil {
			return held, err
		}
		paths = paths[count:]
	}
	return held, nil
}

// nativeAppRunning reports whether name, such as "Cursor", runs as
// Cursor.exe.
func nativeAppRunning(table []ProcessInfo, name string) bool {
	for _, process := range table {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(process.Command), filepath.Ext(process.Command)), name) {
			return true
		}
	}
	return false
}

// nativeHoldersUnder is HoldersUnder through the native reader: every
// file under dir, named under dir as given.
func nativeHoldersUnder(dir string) (map[string]int, error) {
	// The trailing separator makes the walk enter dir when it is a link
	// or a junction, which EvalSymlinks does not resolve.
	var files []string
	err := filepath.WalkDir(filepath.Clean(dir)+string(filepath.Separator), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		return map[string]int{}, err
	}
	return nativeProcesses.holders(files)
}

// hidesClaudeStart reports whether process is a Claude Code runtime whose
// start this user may not read, as for a process of another account. It
// counts as the process a session file names; any other program on that
// pid, such as a service that took a crashed session's pid, does not.
func hidesClaudeStart(process ProcessInfo) bool {
	return process.Started == "" && (process.Command == "claude" || process.Command == "node")
}

// commandName is the program a command line runs: the base name of its
// first word, which Windows may quote, without a Windows .exe.
func commandName(line string) string {
	line = strings.TrimSpace(line)
	first, _, _ := strings.Cut(line, " ")
	if rest, quoted := strings.CutPrefix(line, `"`); quoted {
		first, _, _ = strings.Cut(rest, `"`)
	}
	return withoutExe(filepath.Base(strings.ReplaceAll(first, `\`, "/")))
}

// withoutExe drops a Windows program's .exe, in any case.
func withoutExe(name string) string {
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		return name[:len(name)-len(".exe")]
	}
	return name
}
