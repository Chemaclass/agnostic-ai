package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runScopeCmd runs a command that only records its scope, and returns
// what it resolved with stdout, stderr and the error.
func runScopeCmd(t *testing.T, args ...string) (user bool, stdout, stderr string, err error) {
	t.Helper()
	cmd := &cobra.Command{
		Use:           "probe",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(*cobra.Command, []string) error { return nil },
	}
	addScopeFlag(cmd, &user, "probe the user level")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return user, out.String(), errOut.String(), err
}

func TestScopeFlag_ResolvesUserLevel(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"default is project", nil, false},
		{"project", []string{"--scope", "project"}, false},
		{"user", []string{"--scope", "user"}, true},
		{"global alias", []string{"--global"}, true},
		{"global alias with user", []string{"--global", "--scope", "user"}, true},
		{"global alias switched off", []string{"--global=false"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, stdout, _, err := runScopeCmd(t, tc.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user != tc.want {
				t.Errorf("user=%t, want %t", user, tc.want)
			}
			if stdout != "" {
				t.Errorf("stdout=%q, want empty", stdout)
			}
		})
	}
}

func TestScopeFlag_GlobalPrintsOneDeprecationLineOnStderr(t *testing.T) {
	_, stdout, stderr, err := runScopeCmd(t, "--global")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout=%q, want empty", stdout)
	}
	if got := strings.Count(stderr, "\n"); got != 1 {
		t.Errorf("stderr has %d lines, want 1: %q", got, stderr)
	}
	if !strings.Contains(stderr, "--global") || !strings.Contains(stderr, "--scope user") {
		t.Errorf("stderr=%q, want it to name --global and --scope user", stderr)
	}
}

func TestScopeFlag_ScopePrintsNothingOnStderr(t *testing.T) {
	_, _, stderr, err := runScopeCmd(t, "--scope", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr=%q, want empty", stderr)
	}
}

func TestScopeFlag_RejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"unknown value", []string{"--scope", "bogus"}, []string{"user", "project", "local", `"bogus"`}},
		{"global with project", []string{"--global", "--scope", "project"}, []string{"--global", "--scope project"}},
		{"global with local", []string{"--global", "--scope", "local"}, []string{"--global", "--scope local"}},
		{"local is not supported yet", []string{"--scope", "local"}, []string{"probe", "--scope local"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := runScopeCmd(t, tc.args...)
			if err == nil {
				t.Fatalf("want an error for %v", tc.args)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

// import --scope user takes no target, so its Args check reads the scope
// before the command runs.
func TestScopeFlag_SetsTheScopeBeforeArgsAreChecked(t *testing.T) {
	for _, args := range [][]string{{"--scope", "user"}, {"--global"}} {
		var user, seen bool
		cmd := &cobra.Command{
			Use:  "probe",
			Args: func(*cobra.Command, []string) error { seen = user; return nil },
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		addScopeFlag(cmd, &user, "probe the user level")
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
		if !seen {
			t.Errorf("%v: the Args check ran before the scope was set", args)
		}
	}
}

func TestScopeFlag_HidesGlobalFromHelp(t *testing.T) {
	_, stdout, _, err := runScopeCmd(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "--global") {
		t.Errorf("help lists the deprecated flag:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--scope") {
		t.Errorf("help lacks --scope:\n%s", stdout)
	}
}

// No command runs at local scope yet, so completion does not offer it.
func TestScopeFlag_CompletesTheScopesCommandsSupport(t *testing.T) {
	_, stdout, _, err := runScopeCmd(t, cobra.ShellCompNoDescRequestCmd, "--scope", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{specScopeUser, specScopeProject} {
		if !strings.Contains(stdout, want+"\n") {
			t.Errorf("completion %q lacks %q", stdout, want)
		}
	}
	if strings.Contains(stdout, specScopeLocal+"\n") {
		t.Errorf("completion %q offers a scope every command rejects", stdout)
	}
}

func TestScopeFlag_KeepsAnEarlierPreRun(t *testing.T) {
	var ran, user bool
	cmd := &cobra.Command{
		Use:     "probe",
		PreRunE: func(*cobra.Command, []string) error { ran = true; return nil },
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	addScopeFlag(cmd, &user, "probe the user level")
	cmd.SetArgs([]string{"--scope", "user"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran || !user {
		t.Errorf("ran=%t user=%t, want both true", ran, user)
	}
}

// scopedCommands are the commands that took --global before --scope.
var scopedCommands = [][]string{
	{"sync"}, {"list"}, {"import"}, {"migrate"}, {"validate"}, {"lint"},
	{"explain"}, {"install-hook"}, {"view"}, {"serve"}, {"mcp", "tools"},
}

func TestScopeFlag_EveryFormerGlobalCommandTakesScopeAndHidesGlobal(t *testing.T) {
	root := NewRootCmd("test")
	for _, path := range scopedCommands {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Errorf("%v: %v", path, err)
			continue
		}
		if _, ok := cmd.Flags().Lookup(scopeFlagName).Value.(scopeValue); !ok {
			t.Errorf("%v: --scope is not registered by addScopeFlag", path)
		}
		legacy := cmd.Flags().Lookup(legacyGlobalFlagName)
		if legacy == nil || !legacy.Hidden {
			t.Errorf("%v: --global must stay as a hidden alias, got %+v", path, legacy)
		}
	}
}

// runSplit runs the root command and keeps stdout and stderr apart.
func runSplit(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestScopeFlag_GlobalAndScopeUserPrintTheSameStdout(t *testing.T) {
	userConfigTestHome(t)
	const notice = "warning: --global is deprecated; use --scope user\n"
	for _, args := range [][]string{
		{"list"},
		{"list", "--json"},
		{"validate"},
		{"lint"},
		{"migrate", "--dry-run"},
		{"sync", "--dry-run", "--only", "claude"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			newOut, newErr, err := runSplit(t, append(args, "--scope", "user")...)
			if err != nil {
				t.Fatalf("--scope user: %v\n%s", err, newErr)
			}
			oldOut, oldErr, err := runSplit(t, append(args, "--global")...)
			if err != nil {
				t.Fatalf("--global: %v\n%s", err, oldErr)
			}
			if oldOut != newOut {
				t.Errorf("stdout differs\n--global:\n%s\n--scope user:\n%s", oldOut, newOut)
			}
			if strings.Contains(newErr, "deprecated") {
				t.Errorf("--scope user warned: %q", newErr)
			}
			if strings.Count(oldErr, notice) != 1 || strings.Replace(oldErr, notice, "", 1) != newErr {
				t.Errorf("--global stderr must be the --scope user stderr plus one notice\n--global:\n%s\n--scope user:\n%s", oldErr, newErr)
			}
		})
	}
}

func TestScopeFlag_QuietDropsTheDeprecationNotice(t *testing.T) {
	userConfigTestHome(t)
	// --quiet sets the package verbosity, which later tests read.
	before := verbosity
	t.Cleanup(func() { verbosity = before })
	_, stderr, err := runSplit(t, "list", "--global", "--quiet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stderr, "deprecated") {
		t.Errorf("--quiet printed the notice: %q", stderr)
	}
}

// User-level runs skip the upgrade and recovery prompts. Both spellings
// of the scope must skip them.
func TestScopeFlag_UserScopeSkipsUpgradeAndRecoveryPrompts(t *testing.T) {
	for _, name := range []string{envNoUpdateCheck, envUpgradeInProgress, "CI", "AGNOSTIC_AI_TARGET"} {
		t.Setenv(name, "")
	}
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"--scope", "project"}, true},
		{[]string{"--scope", "user"}, false},
		{[]string{"--global"}, false},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var user bool
			cmd := &cobra.Command{Use: "sync"}
			addScopeFlag(cmd, &user, "probe the user level")
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			if got := automaticUpgradeAllowed(cmd, "0.81.0", true); got != tc.want {
				t.Errorf("automaticUpgradeAllowed=%t, want %t", got, tc.want)
			}
			if got := errorRecoveryAllowed(cmd, true); got != tc.want {
				t.Errorf("errorRecoveryAllowed=%t, want %t", got, tc.want)
			}
		})
	}
}

func TestScopeFlag_GlobalSwitchedOffPointsAtProject(t *testing.T) {
	_, _, stderr, err := runScopeCmd(t, "--global=false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "--scope project") || strings.Contains(stderr, "--scope user") {
		t.Errorf("stderr=%q, want it to point at --scope project", stderr)
	}
}

func TestScopeFlag_GlobalSwitchedOffWithScopeUserPointsAtUser(t *testing.T) {
	user, _, stderr, err := runScopeCmd(t, "--global=false", "--scope", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !user || !strings.Contains(stderr, "--scope user") {
		t.Errorf("user=%t stderr=%q, want user scope and a pointer to --scope user", user, stderr)
	}
}
