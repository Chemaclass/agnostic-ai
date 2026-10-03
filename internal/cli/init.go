package cli

import (
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// defaultBaseDir is the default parent directory for scaffolded source
// folders (agents, skills, rules, hooks, mcps, commands).
const defaultBaseDir = ".agnostic-ai"

// demoFS holds the example specs `init --demo` seeds: one minimal sample
// per source kind, plus the memory-curator skill and the script the demo
// hook runs. A fresh project can run `sync` immediately and see what each
// adapter produces.
//
//go:embed initdata/agents/* initdata/skills/* initdata/rules/* initdata/hooks/* initdata/mcps/* initdata/scripts/*
var demoFS embed.FS

func newInitCmd() *cobra.Command {
	var demo, all, dryRun bool
	gitignore := switchValue(true)
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
			"The prompt starts with the CLIs the project already uses ticked, else the CLIs found on PATH, " +
			"else claude and codex. With no terminal and nothing piped, init enables that same set and prints which it picked. " +
			"In a terminal, when the project has existing tool config, init offers to import it as --from all does. " +
			"The managed .gitignore block is on by default; pass --gitignore=off to commit generated outputs instead. " +
			"Pass --demo to seed example specs: a minimal one per source folder, plus the memory-curator skill. " +
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
  agnostic-ai init --all --gitignore=off

  # Seed example specs, one per source folder plus the memory-curator skill
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
		Args: func(cmd *cobra.Command, args []string) error {
			// --gitignore is a switch, so `--gitignore off` leaves off as
			// the [dir] argument. Which word was meant is ambiguous, so
			// ask for the = form instead of guessing.
			if cmd.Flags().Changed("gitignore") {
				for _, a := range args {
					if _, err := parseSwitch(a); err == nil {
						return fmt.Errorf("%q after --gitignore is ambiguous: pass the value as --gitignore=on or --gitignore=off, or the folder as ./%s", a, a)
					}
				}
			}
			return cobra.MaximumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			base := defaultBaseDir
			if len(args) == 1 {
				base = args[0]
			}
			if preset != "" {
				if err := validatePresetName(preset); err != nil {
					return err
				}
			}
			if !dryRun {
				lock, err := acquireProjectLock(".", "init")
				if err != nil {
					return err
				}
				defer func() { _ = lock.Close() }()
			}
			targets := allTargetNames()
			if !all {
				defaults, kind := initDefaultTargets(".")
				picked, err := selectTargetsForSync(cmd.InOrStdin(), cmd.ErrOrStderr(), defaults)
				if err != nil {
					return err
				}
				targets = picked
				if len(targets) == 0 {
					targets = fallbackInitTargets(cmd.ErrOrStderr(), defaults, kind)
				}
			}
			gitignoreEnabled, gitignoreDefaulted, err := resolveGitignoreChoice(cmd, all, bool(gitignore))
			if err != nil {
				return err
			}
			if fromCLI == "" && !all && !dryRun {
				fromCLI, err = offerExistingImport(".", stdinIsTerminal(cmd.InOrStdin()), promptImportExisting)
				if err != nil {
					return err
				}
			}
			opts := scaffoldOptions{
				Root:             ".",
				Base:             base,
				Targets:          targets,
				Preset:           preset,
				Demo:             demo,
				DryRun:           dryRun,
				GitignoreEnabled: gitignoreEnabled,
				Version:          cmd.Root().Version,
			}
			// The import preview needs the scaffold a real run would write,
			// and a real run stops on an existing project.
			if dryRun && fromCLI != "" {
				if err := ensureNoExistingConfig(".", config.ConfigFileName); err != nil {
					return err
				}
			}
			if err := scaffold(opts); err != nil {
				return err
			}
			if gitignoreDefaulted && !dryRun && verbosity >= levelDefault {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), gitignoreDefaultNote)
			}
			if fromCLI == "" {
				return nil
			}
			if dryRun {
				opts.DryRun = false
				return dryRunImport([]string{fromCLI}, func() error { return scaffoldSilently(opts) }, false)
			}
			cfg, err := config.Load(".")
			if err != nil {
				return fmt.Errorf("load config after init: %w", err)
			}
			run := func() error {
				return withImportTree(".", func() error {
					return withLocalImportGuard(".", cfg, func() error {
						return runImport(".", fromCLI, cfg)
					})
				})
			}
			return runGuardedImport(false, func([]string) string {
				return importOverwriteRemedy([]string{fromCLI})
			}, run)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false,
		"Seed example specs, one per source folder plus the memory-curator skill.")
	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Skip the target picker and enable every supported target.")
	cmd.Flags().StringVar(&preset, "preset", "",
		"Seed stack-flavored starter specs (go, ts-react, python). Composes with --demo and --all.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Print files that would be scaffolded without writing.")
	cmd.Flags().StringVar(&fromCLI, "from", "",
		"After scaffolding, import existing config from this CLI (e.g. claude, cursor, all).")
	cmd.Flags().Var(&gitignore, "gitignore",
		"on or off: persist gitignore.enabled so sync keeps a managed .gitignore block of every emitted target path. On by default; pass --gitignore=off to commit generated outputs instead. When unset and stdin is a TTY, init prompts (defaulting to on).")
	cmd.Flags().Lookup("gitignore").NoOptDefVal = "on"
	_ = cmd.RegisterFlagCompletionFunc("preset", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return availablePresets(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// initDefaultTargets picks the targets init starts from when the user
// names none: the CLIs the project already uses, else the CLIs found on
// PATH, else claude and codex. kind names the tier for the notice. The
// terminal picker pre-ticks this set and the non-terminal path enables
// it, so both start from the same choice.
func initDefaultTargets(root string) (targets []string, kind string) {
	if detected := detectExistingTargets(root); len(detected) > 0 {
		return detected, "detected"
	}
	if installed := installedCLITargets(); len(installed) > 0 {
		return installed, "installed"
	}
	return []string{"claude", "codex"}, "default"
}

// fallbackInitTargets returns the initDefaultTargets choice for an init
// that got no selection: stdin is not a terminal and nothing was piped.
// Never every target: --all is the explicit opt-in for that. One stderr
// line names the choice so a CI log shows what was enabled and how to
// change it. A root AGENTS.md adds a hint to enable codex, which owns
// that file.
func fallbackInitTargets(stderr io.Writer, targets []string, kind string) []string {
	if verbosity < levelDefault {
		return targets // --quiet: errors only
	}
	noun := "targets"
	if len(targets) == 1 {
		noun = "target"
	}
	_, _ = fmt.Fprintf(stderr,
		"no target list piped; enabled %d %s %s: %s (pass --all, or pipe \"claude,codex\")\n",
		len(targets), kind, noun, strings.Join(targets, ", "))
	if !slices.Contains(targets, "codex") && regularFileInside(".", claudeAgentsMainFile) {
		_, _ = fmt.Fprintf(stderr, "  hint: %s exists; enable codex so sync manages it (pipe %q)\n",
			claudeAgentsMainFile, strings.Join(append(slices.Clone(targets), "codex"), ","))
	}
	return targets
}

// offerExistingImport returns "all" when the user agrees to import the
// tool config the project already has, as init --from all does. Without
// a terminal it never imports: the scaffold summary lists the import
// commands instead, so a CI run does not copy config nobody reviewed.
func offerExistingImport(root string, terminal bool, confirm func(sources []string) (bool, error)) (string, error) {
	if !terminal {
		return "", nil
	}
	sources, _ := detectImportSources(root)
	if len(sources) == 0 && regularFileInside(root, filepath.Join(root, claudeAgentsMainFile)) {
		sources = []string{claudeAgentsMainFile}
	}
	if len(sources) == 0 {
		return "", nil
	}
	ok, err := confirm(sources)
	if err != nil || !ok {
		return "", err
	}
	return "all", nil
}

// promptImportExisting asks whether init should import the config it
// found, defaulting to yes.
func promptImportExisting(sources []string) (bool, error) {
	picked := true
	form := huh.NewConfirm().
		Title("Import existing config from " + strings.Join(sources, ", ") + "?").
		Description("Same as agnostic-ai init --from all: copies it into specs under the source folders.").
		Affirmative("Import").
		Negative("Skip").
		Value(&picked)
	if err := form.Run(); err != nil {
		return false, err
	}
	return picked, nil
}

// resolveGitignoreChoice picks the effective gitignore.enabled value
// for a single init invocation, and whether it took the default without
// asking. An explicit --gitignore wins; otherwise a terminal asks, and
// --all or no terminal takes the default (ignore), which init then names,
// since the choice decides what a fresh clone holds.
func resolveGitignoreChoice(cmd *cobra.Command, all, flagValue bool) (enabled, defaulted bool, err error) {
	if cmd.Flags().Changed("gitignore") {
		return flagValue, false, nil
	}
	if all || !stdinIsTerminal(cmd.InOrStdin()) {
		return true, true, nil
	}
	enabled, err = promptGitignoreEnable(cmd.InOrStdin())
	return enabled, false, err
}

const gitignoreDefaultNote = "generated files are git-ignored, so each clone needs agnostic-ai sync (pass --gitignore=off to commit them, so a clone works without the tool)"
