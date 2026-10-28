package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCommandsWithoutArgumentsRejectExtraOnes(t *testing.T) {
	cases := [][]string{
		{"sync", "claude", "bogus"},
		{"sync", "--plan", "claude", "bogus"},
		{"status", "bogus"},
		{"list", "skills", "rules"},
		{"validate", "bogus"},
		{"cleanup", "bogus"},
		{"graph", "bogus"},
		{"upgrade", "bogus"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupEmptyProject(t)
			testutil.Chdir(t, dir)
			silence(t)

			_, err := runRoot(t, args...)

			if err == nil || !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("err = %v, want an unknown command error", err)
			}
		})
	}
}

func TestUpgrade_VersionArgumentPointsAtTheVersionFlag(t *testing.T) {
	for _, args := range [][]string{{"upgrade", "v0.56.1"}, {"upgrade", "0.56.1", "--check"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runRoot(t, args...)

			if err == nil || !strings.Contains(err.Error(), "did you mean --version "+args[1]+"?") {
				t.Fatalf("err = %v, want a pointer to --version", err)
			}
		})
	}
}

func TestGraph_RejectsUnknownKindAndTarget(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"graph", "--kind", "skills"}, `unknown --kind "skills" (did you mean skill?)`},
		{[]string{"graph", "--target", "claud"}, "unknown target: claud (did you mean claude?)"},
		{[]string{"graph", "--target", "bogus"}, "unknown target: bogus"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			dir := setupEmptyProject(t)
			testutil.Chdir(t, dir)
			silence(t)

			_, err := runRoot(t, c.args...)

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestGraph_TargetFilterAcceptsAnyAdapterWhenNoTargetsAreConfigured(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	silence(t)

	if _, err := runRoot(t, "graph", "--target", "claude", "--format", "json"); err != nil {
		t.Fatalf("graph --target claude with no configured targets: %v", err)
	}
}
