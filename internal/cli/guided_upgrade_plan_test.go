package cli

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGuidedUpgrade_SyncPlanRunsWithoutUpgradeSideEffects(t *testing.T) {
	for _, args := range [][]string{{"--plan"}, {"--against", "HEAD", "--plan"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			for _, name := range []string{envNoUpdateCheck, envUpgradeInProgress, "CI", "AGNOSTIC_AI_TARGET"} {
				t.Setenv(name, "")
			}
			dir, git := gitRepo(t)
			testutil.Chdir(t, dir)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nrequires: '>=0.81.0'\n")
			mustWriteFile(t, ".agnostic-ai/rules/demo.md", "demo rule\n")
			git("add", "-A")
			git("commit", "-q", "-m", "project")
			before, err := os.ReadFile("agnostic-ai.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cmd := newSyncCmd()
			var out bytes.Buffer
			input := &planUpgradeInput{reader: strings.NewReader("\n")}
			cmd.SetIn(input)
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			original := cmd.RunE
			var calls []string
			deps := guidedUpgradeDeps{
				check: func(string) (upgradeOffer, error) {
					calls = append(calls, "network")
					return upgradeOffer{Latest: "0.82.0"}, nil
				},
				project:   func() (string, error) { calls = append(calls, "project"); return dir, nil },
				install:   func(io.Writer, string, string) (string, error) { calls = append(calls, "install"); return "/tool", nil },
				reconcile: func(string, string) error { calls = append(calls, "config"); return nil },
				run: func(_, _ string, _ []string, _ io.Writer) (string, error) {
					calls = append(calls, "updated command")
					return "agnostic-ai version 0.82.0", nil
				},
			}
			originalRan := false
			cmd.RunE = func(c *cobra.Command, arguments []string) error {
				if automaticUpgradeAllowed(c, "0.81.0", true) {
					handled, err := runGuidedUpgrade(c, "0.81.0", deps)
					if handled || err != nil {
						return err
					}
				}
				originalRan = true
				return original(c, arguments)
			}
			err = cmd.Execute()
			if len(args) > 1 {
				if err == nil || !strings.Contains(err.Error(), "--against requires --check") {
					t.Errorf("original against validation: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(calls) != 0 {
				t.Errorf("upgrade callbacks ran during plan: %v", calls)
			}
			if input.reads != 0 {
				t.Errorf("upgrade prompt read stdin %d times", input.reads)
			}
			if !originalRan || len(args) == 1 && !strings.Contains(out.String(), "added:") {
				t.Errorf("original plan did not run: %s", out.String())
			}
			after, err := os.ReadFile("agnostic-ai.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Error("plan changed project config")
			}
			if _, err := os.Stat("CLAUDE.md"); !os.IsNotExist(err) {
				t.Errorf("plan created native output: %v", err)
			}
		})
	}
}

type planUpgradeInput struct {
	reader io.Reader
	reads  int
}

func (r *planUpgradeInput) Read(p []byte) (int, error) { r.reads++; return r.reader.Read(p) }
