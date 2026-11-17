package conversations

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// environmentReader on Linux reads /proc/<pid>/environ, whose entries
// are split by NUL bytes.
var environmentReader environmentRead = procEnvironments

// procEnvironments reads each pid's environ file. A pid that no longer
// runs has none.
func procEnvironments(pids []int) (map[int][]string, error) {
	environments := map[int][]string{}
	for _, pid := range pids {
		raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		environments[pid] = claudeEntries(strings.Split(string(raw), "\x00"))
	}
	return environments, nil
}
