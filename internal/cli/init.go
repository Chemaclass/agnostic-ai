package cli

import (
	"embed"

	"github.com/spf13/cobra"
)

// defaultBaseDir is the default parent directory for scaffolded source
// folders (agents, skills, rules, hooks, mcps).
const defaultBaseDir = ".agnostic-ai"

// demoFS holds one minimal sample spec per source kind. Used by
// `init --demo` to seed a fresh project so the user can run `sync`
// immediately and see what each adapter produces.
//
//go:embed initdata/agents/* initdata/skills/* initdata/rules/* initdata/hooks/* initdata/mcps/*
var demoFS embed.FS

func newInitCmd() *cobra.Command {
	var demo, all bool
	var preset string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		Long: "Creates agnostic-ai.yaml plus source folders. " +
			"Default base dir is .agnostic-ai/. Pass a positional argument " +
			"to override (use \".\" for the legacy root-level layout). " +
			"When stdin is a terminal, init prompts for which targets to enable; " +
			"pipe a comma-separated list to skip the prompt, or pass --all / -a " +
			"to enable every supported target without prompting. " +
			"Pass --demo to seed each source folder with a minimal example spec. " +
			"Pass --preset <name> to seed idiomatic specs for a stack (go, ts-react, python).",
		Example: `  # Default: scaffold under .agnostic-ai/, prompt for targets when TTY
  agnostic-ai init

  # Skip the prompt, enable every supported target
  agnostic-ai init --all

  # Non-interactive: pipe the target list
  echo "claude,codex" | agnostic-ai init

  # Seed each source folder with one minimal example spec
  agnostic-ai init --demo

  # Seed idiomatic specs for a stack
  agnostic-ai init --preset go
  agnostic-ai init --preset ts-react

  # Legacy root-level layout (agents/, skills/, rules/, ... at project root)
  agnostic-ai init .

  # Custom base directory
  agnostic-ai init config/ai`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := defaultBaseDir
			if len(args) == 1 {
				base = args[0]
			}
			if preset != "" {
				if err := validatePresetName(preset); err != nil {
					return err
				}
			}
			targets := allTargetNames()
			if !all {
				picked, err := selectTargetsForSync(cmd.InOrStdin(), cmd.ErrOrStderr(), detectExistingTargets("."))
				if err != nil {
					return err
				}
				if len(picked) > 0 {
					targets = picked
				}
			}
			return scaffold(".", base, demo, preset, targets)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false,
		"Seed each source folder with a minimal example spec.")
	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Skip the target picker and enable every supported target.")
	cmd.Flags().StringVar(&preset, "preset", "",
		"Seed stack-flavored starter specs (go, ts-react, python). Composes with --demo and --all.")
	_ = cmd.RegisterFlagCompletionFunc("preset", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return availablePresets(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}
