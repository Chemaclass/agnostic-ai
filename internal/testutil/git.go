package testutil

import (
	"os"
	"strconv"
)

var quietGitConfig = [][2]string{
	{"maintenance.auto", "false"},
	{"gc.auto", "0"},
	{"core.fsmonitor", "false"},
}

// DisableGitBackgroundWork stops every git command the test process runs,
// including those started by the code under test, from leaving detached
// maintenance, gc, or fsmonitor work that writes into a repository while
// t.TempDir cleanup removes it. It replaces any inherited GIT_CONFIG_COUNT
// entries, so call it once from TestMain.
func DisableGitBackgroundWork() error {
	for i, kv := range quietGitConfig {
		n := strconv.Itoa(i)
		if err := os.Setenv("GIT_CONFIG_KEY_"+n, kv[0]); err != nil {
			return err
		}
		if err := os.Setenv("GIT_CONFIG_VALUE_"+n, kv[1]); err != nil {
			return err
		}
	}
	return os.Setenv("GIT_CONFIG_COUNT", strconv.Itoa(len(quietGitConfig)))
}
