package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The binary that offers an upgrade is the old release, so a fix to how
// requires is rewritten would only apply one upgrade later if the old
// binary rewrote it. The updated binary must do the rewrite.
func TestGuidedUpgrade_UpdatedBinaryReconcilesRequires(t *testing.T) {
	root, err := filepath.EvalSymlinks(testutil.TempCwd(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	const original = "version: 1\ntargets: [claude]\nrequires: \">=0.81.0\"\n"
	mustWriteFile(t, "agnostic-ai.yaml", original)
	deps := defaultGuidedUpgradeDeps()
	deps.check = func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }
	deps.manualInstall = nil
	deps.install = func(io.Writer, string, string) (string, error) { return "/updated/tool", nil }
	var commands []string
	deps.run = func(path, dir string, args []string, _ io.Writer) (string, error) {
		if path != "/updated/tool" {
			t.Errorf("ran %q, want the updated binary", path)
		}
		if resolved, _ := filepath.EvalSymlinks(dir); len(args) > 1 && resolved != root {
			t.Errorf("%v ran in %q, want %q", args, dir, root)
		}
		commands = append(commands, strings.Join(args, " "))
		return "agnostic-ai version 0.82.0", nil
	}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(&bytes.Buffer{})
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	want := []string{"--version", "upgrade --requires --guided", "migrate --dry-run", "migrate", "sync", "sync --check"}
	if !reflect.DeepEqual(commands, want) {
		t.Errorf("commands = %q, want %q", commands, want)
	}
	if data, _ := os.ReadFile("agnostic-ai.yaml"); string(data) != original {
		t.Errorf("old binary rewrote the config: %s", data)
	}
}

func TestUpgradeRequiresGuided_RewritesWithoutSync(t *testing.T) {
	requiresProject(t, ">=0.76.0")
	if err := executeUpgradeRequiresForTest(t, "v0.77.0", "--guided"); err != nil {
		t.Fatalf("upgrade --requires --guided: %v", err)
	}
	if got := string(readUpgradeRequiresFile(t, "agnostic-ai.yaml")); !strings.Contains(got, "requires: \">=0.77.0\"") {
		t.Errorf("requires not raised with its form kept: %s", got)
	}
	assertAbsent(t, filepath.Join(".claude", "rules", "safe.md"))
}

func TestUpgradeGuided_NeedsRequires(t *testing.T) {
	requiresProject(t, "0.76.0")
	setRunningVersion(t, "v0.77.0")
	captureLogOut(t)
	root := NewRootCmd("9.9.9")
	root.SetArgs([]string{"upgrade", "--guided"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--guided needs --requires") {
		t.Errorf("--guided without --requires: %v", err)
	}
}
