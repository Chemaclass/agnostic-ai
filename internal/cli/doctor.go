package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// knownCLIBinaries maps target name to the binary expected on PATH.
var knownCLIBinaries = map[string]string{
	"aider":    "aider",
	"amp":      "amp",
	"claude":   "claude",
	"codex":    "codex",
	"gemini":   "gemini",
	"warp":     "warp",
	"zed":      "zed",
	"opencode": "opencode",
}

// reportInstalledCLIs prints which known AI CLI tools are present on PATH.
func reportInstalledCLIs(cmd *cobra.Command) {
	cmd.Println()
	cmd.Println("Installed AI CLIs:")
	any := false
	for target, bin := range knownCLIBinaries {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		cmd.Printf("  ✓ %s → %s\n", target, path)
		any = true
	}
	if !any {
		cmd.Println("  (none found on PATH)")
	}
}

// reportUnsupportedKinds prints any spec kinds that no enabled target supports,
// using the same logic as lintOrphanKinds in validate but formatted for doctor.
func reportUnsupportedKinds(cmd *cobra.Command, cfg *config.Config) {
	_, b, err := loadProject(".")
	if err != nil {
		return
	}
	issues := lintOrphanKinds(b, cfg.Targets, targetsSupportingKind)
	if len(issues) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Unsupported spec kinds:")
	for _, i := range issues {
		cmd.Printf("  ✗ %s: %s\n", i.Path, i.Message)
	}
}

// reportSpecHealth prints the findings `lint` reports for the project,
// then the accepted coverage note count, and returns the findings.
func reportSpecHealth(cmd *cobra.Command, scope checkScope) ([]lintFinding, error) {
	findings, accepted, err := lintScopeReport(scope)
	if err != nil {
		return nil, err
	}
	defer reportAcceptedCoverageNotes(cmd, scope.cfg, accepted)
	cmd.Println()
	cmd.Println("Spec health:")
	if len(findings) == 0 {
		cmd.Println("  ✓ no lint findings")
		return nil, nil
	}
	for _, f := range findings {
		mark := "!"
		if f.Severity == lintError {
			mark = "✗"
		}
		cmd.Printf("  %s %s\n", mark, f)
	}
	cmd.Printf("  %d finding(s): %d error(s), %d warning(s)\n",
		len(findings), countSeverity(findings, lintError), countSeverity(findings, lintWarn))
	return findings, nil
}

// lintErrorsErr fails doctor on error-severity lint findings. Warnings
// are shown but never fail it, as with `lint` without --strict.
func lintErrorsErr(findings []lintFinding) error {
	if n := countSeverity(findings, lintError); n > 0 {
		return fmt.Errorf("%d lint error(s) in source specs. run `agnostic-ai lint` for details", n)
	}
	return nil
}

var errDoctorNoConfig = errors.New("no config found")

// doctorNextStep prints a prioritized "what to do next" hint based on
// whether drift, lint, hook trust, or packaging findings were found and why the config failed to
// load, if it did. manualOnly means the drift is scope documents in
// manual, which neither sync nor doctor --fix removes. fixOnly means the
// drift is nested CLAUDE.md copies, which only doctor --fix removes.
func doctorNextStep(cmd *cobra.Command, drift, manualOnly, fixOnly bool, manual []string, lintFindings, hookFindings int, configErr error, packagingWarnings ...int) {
	cmd.Println()
	cmd.Println("Next step:")
	if errors.Is(configErr, errDoctorNoConfig) {
		cmd.Println("  No agnostic-ai.yaml found. Run: agnostic-ai init")
		return
	}
	if configErr != nil {
		if entry, ok := errs.Lookup(errs.CodeOf(configErr)); ok && entry.Fix != "" {
			cmd.Println("  " + entry.Fix)
			return
		}
		cmd.Println("  Fix the config error above, then run: agnostic-ai doctor")
		return
	}
	switch {
	case drift && manualOnly:
		cmd.Println("  Delete " + manualRemovalAdvice(manual) + ".")
	case drift && fixOnly:
		cmd.Println("  Remove what a rule already holds: agnostic-ai doctor --fix")
	case drift:
		cmd.Println("  Emit missing or stale files: agnostic-ai sync")
		cmd.Println("  Or reconcile in place:       agnostic-ai doctor --fix")
	}
	if lintFindings > 0 {
		cmd.Println("  Review spec findings: agnostic-ai lint")
	}
	if hookFindings > 0 {
		cmd.Println("  Review Codex hook status: open /hooks in Codex")
	}
	packaging := 0
	if len(packagingWarnings) > 0 {
		packaging = packagingWarnings[0]
	}
	if packaging > 0 {
		cmd.Println("  Review packaging ignore coverage before publishing.")
	}
	if !drift && lintFindings == 0 && hookFindings == 0 && packaging == 0 {
		cmd.Println("  All checks passed. Nothing to do.")
	}
}

// newDoctorMCPCmd is the `doctor mcp` subcommand that runs only the MCP
// command-resolution check.
func newDoctorMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Check that every MCP server's command binary is present on PATH.",
		RunE: func(cmd *cobra.Command, args []string) error {
			reportMCPCommandResolution(cmd)
			return nil
		},
	}
}

// newDoctorConfigCmd is the `doctor config` subcommand that validates config.
func newDoctorConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Validate agnostic-ai.yaml schema and report any issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".")
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}
			if cfg.Version < 1 {
				cmd.PrintErrln("config: version field missing or zero")
				return fmt.Errorf("config invalid")
			}
			cmd.Printf("✓ config valid (version %d, %d target(s))\n", cfg.Version, len(cfg.Targets))
			return nil
		},
	}
}

// newDoctorInstallCmd is the `doctor install` subcommand that reports which
// AI CLI tools are installed.
func newDoctorInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Report which AI CLI tools are present on PATH.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("AI CLI installation check:")
			found := 0
			for target, bin := range knownCLIBinaries {
				path, err := exec.LookPath(bin)
				if err != nil {
					cmd.Printf("  — %s (%s) not found\n", target, bin)
					continue
				}
				cmd.Printf("  ✓ %s → %s\n", target, path)
				found++
			}
			cmd.Printf("\n%d / %d known CLIs installed\n", found, len(knownCLIBinaries))
			return nil
		},
	}
}

// doctorConfigOK returns true when agnostic-ai.yaml loads without error.
func doctorConfigOK() bool {
	_, err := os.Stat(config.ConfigFileName)
	return err == nil
}
