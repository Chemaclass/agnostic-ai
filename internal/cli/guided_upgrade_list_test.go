package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGuidedUpgrade_MigrateListRunsWithoutUpgradeSideEffects(t *testing.T) {
	for _, name := range []string{envNoUpdateCheck, envUpgradeInProgress, "CI", "AGNOSTIC_AI_TARGET"} {
		t.Setenv(name, "")
	}
	for _, args := range [][]string{{"--list"}, {"--list", "--only", "hooks"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.TempCwd(t)
			const originalConfig = "version: 1\ntargets: [claude]\n"
			const originalAgent = "---\nname: demo\ntools: [Read]\n---\nagent\n"
			mustWriteFile(t, "agnostic-ai.yaml", originalConfig)
			mustWriteFile(t, ".agnostic-ai/agents/demo.md", originalAgent)
			cmd := newMigrateCmd()
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
				project:   func() (string, error) { calls = append(calls, "project"); return ".", nil },
				install:   func(io.Writer, string, string) (string, error) { calls = append(calls, "install"); return "/tool", nil },
				reconcile: func(string, string) error { calls = append(calls, "config"); return nil },
				run: func(string, string, []string, io.Writer) (string, error) {
					calls = append(calls, "updated command")
					return "agnostic-ai version 0.82.0", nil
				},
			}
			originalRan := false
			cmd.RunE = func(c *cobra.Command, args []string) error {
				if automaticUpgradeAllowed(c, "0.81.0", true) {
					handled, err := runGuidedUpgrade(c, "0.81.0", deps)
					if handled || err != nil {
						return err
					}
				}
				originalRan = true
				return original(c, args)
			}
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 0 || input.reads != 0 {
				t.Errorf("list ran upgrade calls=%v reads=%d", calls, input.reads)
			}
			if !originalRan || !strings.Contains(out.String(), "hooks-portable-events") {
				t.Errorf("original listing did not run: %s", out.String())
			}
			for path, want := range map[string]string{"agnostic-ai.yaml": originalConfig, ".agnostic-ai/agents/demo.md": originalAgent} {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != want {
					t.Errorf("%s changed: %s", path, data)
				}
			}
			if _, err := os.Stat(".agnostic-ai/" + projectLockName); !os.IsNotExist(err) {
				t.Errorf("list created project lock: %v", err)
			}
		})
	}
	cmd := newMigrateCmd()
	if err := cmd.ParseFlags([]string{"--list=false"}); err != nil {
		t.Fatal(err)
	}
	if !automaticUpgradeAllowed(cmd, "0.81.0", true) {
		t.Error("--list=false suppressed normal migration upgrade offer")
	}
}
