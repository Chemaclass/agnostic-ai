package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func recoveryTestCommand(input string) (*cobra.Command, *bytes.Buffer) {
	root := &cobra.Command{Use: "agnostic-ai"}
	cmd := &cobra.Command{Use: "ui"}
	root.AddCommand(cmd)
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	return cmd, &out
}

func recoveryTestDeps(t *testing.T) errorRecoveryDeps {
	t.Helper()
	return errorRecoveryDeps{
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		apply: func() error {
			t.Error("applied a fix without selecting it")
			return nil
		},
		launch: func(*cobra.Command, string, string) error {
			t.Error("launched a tool without selecting it")
			return nil
		},
	}
}

func TestErrorRecovery_OnlyOffersConfiguredInstalledCLIs(t *testing.T) {
	setRunningVersion(t, "v0.89.0")
	cmd, out := recoveryTestCommand("0\n")
	deps := recoveryTestDeps(t)
	deps.lookPath = func(name string) (string, error) {
		if name == "codex" {
			return "/tools/codex", nil
		}
		return "", exec.ErrNotFound
	}
	cfg := &config.Config{Requires: "0.88.0", Targets: []string{"claude", "codex", "cursor", "missing"}}
	handled, err := runErrorRecovery(cmd, requireVersion("agnostic-ai.yaml", cfg.Requires), cfg.Requires, cfg.Targets, deps)
	if handled || err != nil {
		t.Errorf("exit = (%v, %v), want unhandled original error", handled, err)
	}
	for _, want := range []string{"Apply suggested fix", "Open Codex", "Show the fix prompt to copy", "0. Exit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("menu missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Open Claude", "Open Cursor", "Open missing"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("menu offered unavailable tool %q:\n%s", unwanted, out)
		}
	}
}

func TestErrorRecovery_AppliesTheExistingRequiresFix(t *testing.T) {
	requiresProject(t, "0.88.0")
	setRunningVersion(t, "v0.89.0")
	captureLogOut(t)
	cmd, out := recoveryTestCommand("1\n")
	deps := recoveryTestDeps(t)
	deps.apply = runUpgradeRequires
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	handled, err := runErrorRecovery(cmd, requireVersion("agnostic-ai.yaml", cfg.Requires), cfg.Requires, cfg.Targets, deps)
	if !handled || err != nil {
		t.Fatalf("apply = (%v, %v), want successful fix", handled, err)
	}
	cfg, err = config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Requires != "0.89.0" {
		t.Errorf("requires = %q, want 0.89.0", cfg.Requires)
	}
	if _, _, err := runAsVersion(t, "v0.89.0", "sync", "--check"); err != nil {
		t.Errorf("recovery did not sync: %v", err)
	}
	if !strings.Contains(out.String(), "Rerun your original command") {
		t.Errorf("missing next step: %s", out)
	}
}

func TestErrorRecovery_UIFixUsesTheExplicitProjectDirectory(t *testing.T) {
	requiresProject(t, "0.88.0")
	setRunningVersion(t, "v0.89.0")
	captureLogOut(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	writeFile(t, filepath.Join(other, config.ConfigFileName), "version: 1\nrequires: \"0.87.0\"\ntargets: [codex]\n")
	root := NewRootCmd("0.89.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("1\n"))
	root.SetArgs([]string{"ui", "--dir", other})
	cmd, original := root.ExecuteC()
	if errs.CodeOf(original) != errs.CodeRequiresUnmet {
		t.Fatalf("ui --dir error = %v, want AAI-005", original)
	}
	deps := recoveryTestDeps(t)
	deps.apply = runUpgradeRequires
	handled, err := runProjectErrorRecovery(cmd, original, deps)
	if !handled || err != nil {
		t.Fatalf("project recovery = (%v, %v), want successful fix", handled, err)
	}
	current, err := config.Load(cwd)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := config.Load(other)
	if err != nil {
		t.Fatal(err)
	}
	if current.Requires != "0.88.0" || updated.Requires != "0.89.0" {
		t.Errorf("updated wrong project: cwd %q, selected %q", current.Requires, updated.Requires)
	}
	if after, err := os.Getwd(); err != nil || after != cwd {
		t.Errorf("working directory = %q, %v; want %q", after, err, cwd)
	}
}

func TestErrorRecovery_UILaunchUsesTheProjectAboveTheCurrentDirectory(t *testing.T) {
	requiresProject(t, "0.88.0")
	setRunningVersion(t, "v0.89.0")
	project, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(project, "src")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	root := NewRootCmd("0.89.0")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader("2\n"))
	root.SetArgs([]string{"ui"})
	cmd, original := root.ExecuteC()
	if errs.CodeOf(original) != errs.CodeRequiresUnmet {
		t.Fatalf("nested ui error = %v, want AAI-005", original)
	}
	deps := recoveryTestDeps(t)
	deps.lookPath = func(name string) (string, error) { return "/tools/" + name, nil }
	launched := false
	deps.launch = func(_ *cobra.Command, path, _ string) error {
		launched = true
		cwd, err := os.Getwd()
		if err != nil || cwd != project || path != "/tools/claude" {
			t.Errorf("launch cwd = %q, %v; CLI = %q", cwd, err, path)
		}
		return nil
	}
	handled, err := runProjectErrorRecovery(cmd, original, deps)
	if !handled || err != nil || !launched {
		t.Errorf("nested recovery = (%v, %v), launched = %v", handled, err, launched)
	}
	if after, err := os.Getwd(); err != nil || after != nested {
		t.Errorf("working directory = %q, %v; want %q", after, err, nested)
	}
}

func TestErrorRecovery_LaunchesTheSelectedCLIWithTheErrorPrompt(t *testing.T) {
	setRunningVersion(t, "v0.89.0")
	cmd, out := recoveryTestCommand("3\n")
	deps := recoveryTestDeps(t)
	deps.lookPath = func(name string) (string, error) { return "/tools/" + name, nil }
	var launched string
	deps.launch = func(_ *cobra.Command, path, prompt string) error {
		launched = path
		for _, want := range []string{"agnostic-ai ui", "AAI-005", "agnostic-ai.local.yaml", "upgrade --requires", "schema URL", "sync --check"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("prompt missing %q:\n%s", want, prompt)
			}
		}
		return nil
	}
	cfg := &config.Config{Requires: "0.88.0", Targets: []string{"claude", "codex"}}
	original := requireVersion("agnostic-ai.yaml + agnostic-ai.local.yaml", cfg.Requires)
	handled, err := runErrorRecovery(cmd, original, cfg.Requires, cfg.Targets, deps)
	if !handled || err != nil || launched != "/tools/codex" {
		t.Errorf("launch = (%v, %v, %q), want selected Codex", handled, err, launched)
	}
	if !strings.Contains(out.String(), "Prompt to send:") {
		t.Errorf("launch did not show the prompt: %s", out)
	}
}

func TestErrorRecovery_PropagatesActionFailures(t *testing.T) {
	setRunningVersion(t, "v0.89.0")
	cmd, _ := recoveryTestCommand("1\n")
	deps := recoveryTestDeps(t)
	want := errors.New("sync failed")
	deps.apply = func() error { return want }
	cfg := &config.Config{Requires: "0.88.0"}
	handled, err := runErrorRecovery(cmd, requireVersion("agnostic-ai.yaml", cfg.Requires), cfg.Requires, cfg.Targets, deps)
	if !handled || !errors.Is(err, want) {
		t.Errorf("apply failure = (%v, %v), want sync failure", handled, err)
	}
}

func TestErrorRecovery_CopyPromptKeepsTheOriginalFailure(t *testing.T) {
	setRunningVersion(t, "v0.88.0")
	cmd, out := recoveryTestCommand("invalid\n1\n")
	cfg := &config.Config{Requires: ">=0.89.0"}
	handled, err := runErrorRecovery(cmd, requireVersion("agnostic-ai.yaml", cfg.Requires), cfg.Requires, cfg.Targets, recoveryTestDeps(t))
	if handled || err != nil {
		t.Errorf("copy = (%v, %v), want original failure", handled, err)
	}
	for _, want := range []string{"Choose a number", "Prompt to copy:", "AAI-005", "agnostic-ai upgrade"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
	if strings.Contains(out.String(), "Apply suggested fix") || strings.Contains(out.String(), "upgrade --requires") {
		t.Errorf("older binary offered project adoption: %s", out)
	}
}

func TestErrorRecovery_EmptyInputDoesNotAct(t *testing.T) {
	setRunningVersion(t, "v0.89.0")
	for _, input := range []string{"", "\n", "0\n"} {
		cmd, _ := recoveryTestCommand(input)
		cfg := &config.Config{Requires: "0.88.0"}
		handled, err := runErrorRecovery(cmd, requireVersion("agnostic-ai.yaml", cfg.Requires), cfg.Requires, cfg.Targets, recoveryTestDeps(t))
		if handled || err != nil {
			t.Errorf("input %q = (%v, %v), want original failure", input, handled, err)
		}
	}
}

func TestErrorRecovery_SkipsAutomation(t *testing.T) {
	for _, flag := range []string{"quiet", "json", "global", "watch", "check", "dry-run", "plan", "list", "format"} {
		t.Run(flag, func(t *testing.T) {
			cmd, _ := recoveryTestCommand("")
			if flag == "format" {
				cmd.Flags().String(flag, "json", "")
			} else {
				cmd.Flags().Bool(flag, true, "")
			}
			if errorRecoveryAllowed(cmd, true) {
				t.Error("offered recovery during automation")
			}
		})
	}
	cmd, out := recoveryTestCommand("")
	if errorRecoveryAllowed(cmd, false) {
		t.Error("offered recovery without a terminal")
	}
	t.Setenv("CI", "true")
	if errorRecoveryAllowed(cmd, true) {
		t.Error("offered recovery in CI")
	}
	if err := OfferErrorRecovery(cmd, errs.Coded(errs.CodeRequiresUnmet, "test")); err != nil || out.Len() != 0 {
		t.Errorf("noninteractive recovery = %v, output %q", err, out)
	}
}

func TestErrorRecovery_LaunchedToolExitStatusIsNotAFailure(t *testing.T) {
	if err := childLaunchFailure(&exec.ExitError{ProcessState: &os.ProcessState{}}); err != nil {
		t.Errorf("child exit status became a failure: %v", err)
	}
	if err := childLaunchFailure(nil); err != nil {
		t.Errorf("clean exit = %v", err)
	}
	if err := childLaunchFailure(exec.ErrNotFound); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("start failure lost: %v", err)
	}
}

func TestErrorRecovery_OffersToolsWhenAnOlderBinaryCannotReadTheConfig(t *testing.T) {
	requiresProject(t, ">=0.90.0")
	writeFile(t, config.ConfigFileName, "version: 1\nrequires: \">=0.90.0\"\ntargets: [codex]\nfuture-key: true\n")
	setRunningVersion(t, "v0.89.0")
	root := NewRootCmd("0.89.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("1\n"))
	root.SetArgs([]string{"ui"})
	cmd, original := root.ExecuteC()
	if errs.CodeOf(original) != errs.CodeRequiresUnmet {
		t.Fatalf("ui error = %v, want AAI-005", original)
	}
	deps := recoveryTestDeps(t)
	deps.lookPath = func(name string) (string, error) { return "/tools/" + name, nil }
	var launched string
	deps.launch = func(_ *cobra.Command, path, _ string) error {
		launched = path
		return nil
	}
	handled, err := runProjectErrorRecovery(cmd, original, deps)
	if !handled || err != nil || launched != "/tools/codex" {
		t.Errorf("recovery = (%v, %v, %q), want Codex launched", handled, err, launched)
	}
	if strings.Contains(out.String(), "Apply suggested fix") {
		t.Errorf("older binary offered project adoption: %s", out.String())
	}
}
