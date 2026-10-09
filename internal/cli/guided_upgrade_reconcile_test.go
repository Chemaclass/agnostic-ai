package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGuidedUpgrade_ReconcileFailureReportsInstalledVersionAndRecovery(t *testing.T) {
	root := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	const original = "{version: 1, requires: 0.81.0, targets: [claude]}\n"
	mustWriteFile(t, "agnostic-ai.yaml", original)
	if _, err := config.Load(root); err != nil {
		t.Fatalf("flow-style config is valid: %v", err)
	}
	deps := defaultGuidedUpgradeDeps()
	deps.check = func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }
	deps.manualInstall = nil
	installed := false
	deps.install = func(io.Writer, string, string) (string, error) { installed = true; return "/updated/tool", nil }
	var commands []string
	deps.run = func(_, _ string, args []string, _ io.Writer) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		return "agnostic-ai version 0.82.0", nil
	}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(&bytes.Buffer{})
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if !handled || !installed || err == nil {
		t.Fatalf("handled=%v installed=%v err=%v", handled, installed, err)
	}
	if len(commands) != 1 || commands[0] != "--version" {
		t.Errorf("commands after reconciliation failed: %v", commands)
	}
	for _, want := range []string{"0.82.0", "installed", "requires", "schema", "migrations did not start", "flow-style", "Resolve", "upgrade --requires", "migrate --dry-run", "migrate", "sync --check", root} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("recovery error lacks %q: %v", want, err)
		}
	}
	data, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("flow config changed: %s", data)
	}
}
