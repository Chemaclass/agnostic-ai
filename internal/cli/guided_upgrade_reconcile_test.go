package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
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
	deps.run = func(_, dir string, args []string, w io.Writer) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		if reflect.DeepEqual(args, guidedRequiresArgs) {
			// The updated binary prints its error, and the parent sees only the exit status.
			if err := reconcileGuidedRequires(dir, "0.82.0"); err != nil {
				_, _ = fmt.Fprintln(w, err)
				return "", errors.New("exit status 1")
			}
			return "", nil
		}
		return "agnostic-ai version 0.82.0", nil
	}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if !handled || !installed || err == nil {
		t.Fatalf("handled=%v installed=%v err=%v", handled, installed, err)
	}
	if want := []string{"--version", "upgrade --requires --guided"}; !reflect.DeepEqual(commands, want) {
		t.Errorf("commands after reconciliation failed: %v", commands)
	}
	for _, want := range []string{"0.82.0", "installed", "requires", "schema", "migrations did not start", "Resolve", "upgrade --requires", "migrate --dry-run", "migrate", "sync --check", root} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("recovery error lacks %q: %v", want, err)
		}
	}
	if !strings.Contains(out.String(), "flow-style") {
		t.Errorf("output lacks the updated binary's reason: %s", out.String())
	}
	data, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("flow config changed: %s", data)
	}
}
