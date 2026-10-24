package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRoot_HelpListsEveryCommandOnceByTask(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	groups := []string{"Setup:", "Daily generation:", "Diagnosis and inspection:", "Maintenance:"}
	last := -1
	for _, title := range groups {
		pos := strings.Index(out.String(), title)
		if pos <= last {
			t.Errorf("task group %q missing or out of order in help", title)
		}
		last = pos
	}
	if strings.Contains(out.String(), "Additional Commands:") {
		t.Error("help has commands outside the task groups")
	}

	want := strings.Fields("cleanup compare completion doctor explain graph handoff help hook import init install-hook lint list lsp mcp memory migrate new packs project render revert serve status sync upgrade use validate verify view why")
	listed := map[string]int{}
	for line := range strings.SplitSeq(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && strings.HasPrefix(line, "  ") {
			listed[fields[0]]++
		}
	}
	registered := map[string]bool{}
	for _, cmd := range root.Commands() {
		if !cmd.IsAvailableCommand() && cmd.Name() != "help" {
			continue
		}
		registered[cmd.Name()] = true
		if cmd.GroupID == "" || !root.ContainsGroup(cmd.GroupID) {
			t.Errorf("%s has no task group", cmd.Name())
		}
		if listed[cmd.Name()] != 1 {
			t.Errorf("%s appears %d times in help, want once", cmd.Name(), listed[cmd.Name()])
		}
	}
	for _, name := range want {
		if !registered[name] {
			t.Errorf("existing command %s is no longer registered", name)
		}
	}
	if len(registered) != len(want) {
		t.Errorf("got %d registered commands, want %d", len(registered), len(want))
	}
}

func TestRoot_HelpExamplesFollowSetupPlanSyncAndDiagnosis(t *testing.T) {
	examples := NewRootCmd("test").Example
	last := -1
	for _, command := range []string{`echo "claude,codex" | agnostic-ai init`, "agnostic-ai sync --plan", "agnostic-ai sync\n", "agnostic-ai doctor"} {
		pos := strings.Index(examples, command)
		if pos <= last {
			t.Errorf("example %q missing or out of order:\n%s", command, examples)
		}
		last = pos
	}
	if strings.Contains(examples, "every target") {
		t.Error("ordinary init must not claim to select every target")
	}
}

func TestDoctor_HelpStartsWithReadOnlyDiagnosis(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"doctor", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"read-only by default", "next step", "--fix to change files"} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("doctor help must explain %q:\n%s", text, &out)
		}
	}
}

func TestRoot_TaskGroupsKeepHelpAliasAndCompletion(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := NewRootCmd("test")
	for _, cmd := range root.Commands() {
		if !cmd.IsAvailableCommand() {
			continue
		}
		t.Run(cmd.Name(), func(t *testing.T) {
			root := NewRootCmd("test")
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs([]string{cmd.Name(), "--help"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Usage:") {
				t.Errorf("%s help lost usage", cmd.Name())
			}
		})
	}
	alias, _, err := root.Find([]string{"update"})
	if err != nil || alias.Name() != "upgrade" {
		t.Errorf("update alias resolved to %v, %v", alias, err)
	}

	completed := map[string]bool{}
	for line := range strings.SplitSeq(runCompletion(t, ""), "\n") {
		name, _, _ := strings.Cut(line, "\t")
		completed[name] = true
	}
	root.InitDefaultHelpCmd()
	for _, cmd := range root.Commands() {
		if (cmd.IsAvailableCommand() || cmd.Name() == "help") && !completed[cmd.Name()] {
			t.Errorf("%s missing from root completion", cmd.Name())
		}
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out := captureStdout(t, func() {
				root := NewRootCmd("test")
				root.SetArgs([]string{"completion", shell})
				if err := root.Execute(); err != nil {
					t.Error(err)
				}
			})
			if !strings.Contains(out, "agnostic-ai") {
				t.Errorf("%s completion script missing command name", shell)
			}
		})
	}
}
