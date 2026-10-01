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
		{[]string{"--gitignore", "off"}, false},
		{[]string{"--gitignore=off"}, false},
		{[]string{"--gitignore=false"}, false},
		{[]string{"--gitignore", "on"}, true},
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
