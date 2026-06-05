package cli

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// defaultBaseDir is the default parent directory for scaffolded source
// folders (agents, skills, rules, hooks, mcps, commands).
const defaultBaseDir = ".agnostic-ai"

// demoFS holds one minimal sample spec per source kind. Used by
// `init --demo` to seed a fresh project so the user can run `sync`
// immediately and see what each adapter produces.
//
//go:embed initdata/agents/* initdata/skills/* initdata/rules/* initdata/hooks/* initdata/mcps/*
var demoFS embed.FS

func newInitCmd() *cobra.Command {
	var demo, all, dryRun, gitignore bool
	var preset, fromCLI string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		Long: "Creates agnostic-ai.yaml plus source folders. " +
			"Default base dir is .agnostic-ai/. Pass a positional argument " +
			"to override (use \".\" for the legacy root-level layout). " +
			"When stdin is a terminal, init prompts for which targets to enable " +
			"and whether to keep a managed .gitignore block of every emitted target path (default yes); " +
			"pipe a comma-separated list to skip the target prompt, or pass --all / -a " +
			"to skip both prompts and enable every supported target. " +
			"The managed .gitignore block is on by default; pass --gitignore=false to commit generated outputs instead. " +
			"Pass --demo to seed each source folder with a minimal example spec. " +
			"Pass --preset <name> to seed idiomatic specs for a stack (go, ts-react, python). " +
			"Pass --from <cli> to scaffold and then import existing CLI config in one step.",
		Example: `  # Default: scaffold under .agnostic-ai/, prompt for targets when TTY
  agnostic-ai init

  # Scaffold and import existing Claude Code config in one step
  agnostic-ai init --from claude

  # Scaffold and import from every detected AI CLI
  agnostic-ai init --from all

  # Skip the prompt, enable every supported target
  agnostic-ai init --all

  # Non-interactive: pipe the target list
  echo "claude,codex" | agnostic-ai init

  # Commit generated outputs instead of ignoring them
  agnostic-ai init --all --gitignore=false

  # Seed each source folder with one minimal example spec
  agnostic-ai init --demo

  # Seed idiomatic specs for a stack
  agnostic-ai init --preset go
  agnostic-ai init --preset ts-react

  # Preview what would be scaffolded without writing
  agnostic-ai init --dry-run --all

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
			gitignoreEnabled, err := resolveGitignoreChoice(cmd, all, gitignore)
			if err != nil {
				return err
			}
			if err := scaffold(scaffoldOptions{
				Root:             ".",
				Base:             base,
				Targets:          targets,
				Preset:           preset,
				Demo:             demo,
				DryRun:           dryRun,
				GitignoreEnabled: gitignoreEnabled,
			}); err != nil {
				return err
			}
			if fromCLI == "" || dryRun {
				return nil
			}
			cfg, err := config.Load(".")
			if err != nil {
				return fmt.Errorf("load config after init: %w", err)
			}
			return runImport(".", fromCLI, cfg)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false,
		"Seed each source folder with a minimal example spec.")
	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Skip the target picker and enable every supported target.")
	cmd.Flags().StringVar(&preset, "preset", "",
		"Seed stack-flavored starter specs (go, ts-react, python). Composes with --demo and --all.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Print files that would be scaffolded without writing.")
	cmd.Flags().StringVar(&fromCLI, "from", "",
		"After scaffolding, import existing config from this CLI (e.g. claude, cursor, all).")
	cmd.Flags().BoolVar(&gitignore, "gitignore", true,
		"Persist gitignore.enabled so `sync` keeps a managed .gitignore block of every emitted target path. Enabled by default; pass --gitignore=false to commit generated outputs instead. When unset and stdin is a TTY, init prompts (defaulting to yes).")
	_ = cmd.RegisterFlagCompletionFunc("preset", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return availablePresets(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// resolveGitignoreChoice picks the effective gitignore.enabled value
// for a single init invocation. A fresh project ignores its generated
// outputs by default; the source specs under .agnostic-ai/ stay the one
// committed copy and contributors run `sync` locally.
//
//   - an explicit --gitignore / --gitignore=false wins (the typed value sticks),
//   - --all skips the prompt and enables the managed block,
//   - otherwise the TTY confirm prompt drives the choice (defaulting to
//     yes); non-TTY stdin enables it so first-time and CI inits never
//     silently commit generated files.
func resolveGitignoreChoice(cmd *cobra.Command, all, flagValue bool) (bool, error) {
	if cmd.Flags().Changed("gitignore") {
		return flagValue, nil
	}
	if all {
		return true, nil
	}
	return promptGitignoreEnable(cmd.InOrStdin())
}
