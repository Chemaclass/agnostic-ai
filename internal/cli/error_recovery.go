package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

type errorRecoveryDeps struct {
	lookPath func(string) (string, error)
	apply    func() error
	launch   func(*cobra.Command, string, string) error
}

// requiresProjectError carries what the menu needs from the project, since
// an older binary cannot reload a config that uses keys it does not know.
type requiresProjectError struct {
	err      error
	root     string
	requires string
	targets  []string
}

func (e *requiresProjectError) Error() string { return e.err.Error() }
func (e *requiresProjectError) Unwrap() error { return e.err }

func withRequiresProject(err error, root, requires string, targets []string) error {
	abs, pathErr := filepath.Abs(root)
	if pathErr != nil {
		return err
	}
	return &requiresProjectError{err: err, root: abs, requires: requires, targets: targets}
}

// OfferErrorRecovery shows the next-step menu for a project version mismatch.
// The caller keeps the original exit status in every case: the selected
// action never runs the failed command. The returned error is the failure of
// the chosen action, if any.
func OfferErrorRecovery(cmd *cobra.Command, original error) error {
	if cmd == nil || errs.CodeOf(original) != errs.CodeRequiresUnmet || !errorRecoveryEligible(cmd) {
		return nil
	}
	_, err := runProjectErrorRecovery(cmd, original, errorRecoveryDeps{
		lookPath: exec.LookPath,
		apply:    runUpgradeRequires,
		launch:   launchRecoveryCLI,
	})
	return err
}

func runProjectErrorRecovery(cmd *cobra.Command, original error, deps errorRecoveryDeps) (bool, error) {
	project, ok := errors.AsType[*requiresProjectError](original)
	if !ok {
		return false, nil
	}
	if err := refuseGlobalHome(project.root, ""); err != nil {
		return false, nil
	}
	source, err := openAuthoritySource(project.root, false)
	if err != nil {
		return false, err
	}
	defer source.close()
	return runErrorRecovery(cmd, original, project.requires, project.targets, deps)
}

func errorRecoveryEligible(cmd *cobra.Command) bool {
	out, ok := cmd.OutOrStdout().(*os.File)
	errOut, errOK := cmd.ErrOrStderr().(*os.File)
	interactive := stdinIsTerminal(cmd.InOrStdin()) && ok && term.IsTerminal(out.Fd()) && errOK && term.IsTerminal(errOut.Fd())
	return errorRecoveryAllowed(cmd, interactive)
}

func errorRecoveryAllowed(cmd *cobra.Command, interactive bool) bool {
	if !interactive || os.Getenv("CI") != "" || os.Getenv(envUpgradeInProgress) != "" || os.Getenv("AGNOSTIC_AI_TARGET") != "" {
		return false
	}
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "hook", "handoff", "project", "install-hook", "lsp", "completion":
			return false
		}
	}
	for _, name := range []string{"quiet", "json", "global", "watch", "check", "dry-run", "plan", "list"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Value.String() == "true" {
			return false
		}
	}
	if f := cmd.Flags().Lookup("format"); f != nil && f.Value.String() == "json" {
		return false
	}
	return true
}

type errorRecoveryAction struct {
	label string
	path  string
	apply bool
}

func runErrorRecovery(cmd *cobra.Command, original error, requires string, targets []string, deps errorRecoveryDeps) (bool, error) {
	var actions []errorRecoveryAction
	if req, err := config.ParseRequirement(requires); err == nil && req.Above(runningVersion) {
		actions = append(actions, errorRecoveryAction{
			label: "Apply suggested fix (update project requires and schema, then sync)",
			apply: true,
		})
	}
	prompt := errorRecoveryPrompt(cmd.CommandPath(), original)
	for _, target := range targets {
		label := ""
		switch target {
		case "claude":
			label = "Claude Code"
		case "codex":
			label = "Codex"
		default:
			continue
		}
		path, err := deps.lookPath(knownCLIBinaries[target])
		if err != nil {
			continue
		}
		actions = append(actions, errorRecoveryAction{
			label: "Open " + label + " with a fix prompt",
			path:  path,
		})
	}
	actions = append(actions, errorRecoveryAction{label: "Show the fix prompt to copy"})
	cmd.Println("\nChoose a next step:")
	for i, action := range actions {
		cmd.Printf("  %d. %s\n", i+1, action.label)
	}
	cmd.Println("  0. Exit")
	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		cmd.Print("Choice [0]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, nil
		}
		choice := strings.TrimSpace(line)
		if choice == "" || choice == "0" {
			return false, nil
		}
		index, err := strconv.Atoi(choice)
		if err != nil || index < 1 || index > len(actions) {
			cmd.Printf("Choose a number from 0 to %d.\n", len(actions))
			continue
		}
		action := actions[index-1]
		switch {
		case action.apply:
			if err := deps.apply(); err != nil {
				return true, err
			}
			cmd.Println("Project updated. Rerun your original command.")
			return true, nil
		case action.path != "":
			cmd.Println("\nPrompt to send:")
			cmd.Println(prompt)
			if err := deps.launch(cmd, action.path, prompt); err != nil {
				return true, fmt.Errorf("launch %s: %w", action.path, err)
			}
			return true, nil
		default:
			cmd.Println("\nPrompt to copy:")
			cmd.Println(prompt)
			return false, nil
		}
	}
}

func errorRecoveryPrompt(command string, original error) string {
	return "Help me resolve this agnostic-ai error in the current project.\n\n" +
		"Command: " + command + "\n" + ErrorText(original) + "\n\n" +
		"Inspect the project config and local override. Explain the suggested fix and ask before changing a deliberate version pin. " +
		"Preserve other settings, review the diff, and verify with agnostic-ai sync --check."
}

// launchRecoveryCLI hands the terminal to the AI tool. Ctrl-C inside that
// session must reach the child only, so the parent ignores it until the
// child exits. The child's own exit status is not an agnostic-ai failure.
func launchRecoveryCLI(cmd *cobra.Command, path, prompt string) error {
	child := exec.Command(path, prompt)
	child.Stdin = cmd.InOrStdin()
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()
	signal.Ignore(os.Interrupt, syscall.SIGTERM)
	defer signal.Reset(os.Interrupt, syscall.SIGTERM)
	return childLaunchFailure(child.Run())
}

func childLaunchFailure(err error) error {
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		return nil
	}
	return err
}
