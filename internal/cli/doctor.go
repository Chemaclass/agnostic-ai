package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
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
	issues := lintOrphanKinds(b, cfg.Targets)
	if len(issues) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Unsupported spec kinds:")
	for _, i := range issues {
		cmd.Printf("  ✗ %s: %s\n", i.Path, i.Message)
	}
}

// doctorNextStep prints a prioritized "what to do next" hint based on
// whether drift was found and whether the config loaded successfully.
func doctorNextStep(cmd *cobra.Command, drift bool, configOK bool) {
	cmd.Println()
	cmd.Println("Next step:")
	if !configOK {
		cmd.Println("  No agnostic-ai.yaml found. Run: agnostic-ai init")
		return
	}
	if drift {
		cmd.Println("  Emit missing or stale files: agnostic-ai sync")
		cmd.Println("  Or reconcile in place:       agnostic-ai doctor --fix")
		return
	}
	cmd.Println("  All checks passed. Nothing to do.")
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
