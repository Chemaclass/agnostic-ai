package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// `--gitignore off` used to leave off as the [dir] argument, so init
// made a folder named off and kept the managed block on (#1594).
func TestInit_GitignoreTakesOnAndOff(t *testing.T) {
	for _, tc := range []struct {
		args []string
		on   bool
	}{
		{[]string{"--gitignore=off"}, false},
		{[]string{"--gitignore=false"}, false},
		{[]string{"--gitignore"}, true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			if out, err := runCLI(t, append([]string{"init", "--all"}, tc.args...)...); err != nil {
				t.Fatalf("init: %v\n%s", err, out)
			}
			if _, err := os.Stat("off"); err == nil {
				t.Error("init made a folder named off")
			}
			if cfg := readFile(t, "agnostic-ai.yaml"); strings.Contains(cfg, "enabled: true") != tc.on {
				t.Errorf("gitignore on = %v, want %v:\n%s", !tc.on, tc.on, cfg)
			}
		})
	}
}

// A bool-style --gitignore leaves `off` in `--gitignore off` as the
// [dir] argument; init asks for the = form instead of making a folder.
func TestInit_GitignoreWithASpaceAsksForTheEqualsForm(t *testing.T) {
	for _, args := range [][]string{{"--gitignore", "off"}, {"config/ai", "--gitignore", "off"}, {"--gitignore", "on"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			_, err := runCLI(t, append([]string{"init", "--all"}, args...)...)
			if err == nil || !strings.Contains(err.Error(), "--gitignore=") {
				t.Errorf("init error = %v, want the = form", err)
			}
			if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
				t.Error("init wrote a project")
			}
		})
	}
}

func TestInit_GitignoreValueKeepsTheDirArgument(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if out, err := runCLI(t, "init", "--all", "--gitignore=off", "config/ai"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if _, err := os.Stat("config/ai"); err != nil {
		t.Errorf("init lost the dir argument: %v", err)
	}
}
