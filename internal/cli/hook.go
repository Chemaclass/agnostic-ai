package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/hookpaths"
)

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Helpers for commands that run inside a tool's hook",
	}
	cmd.AddCommand(newHookPathsCmd(), newHookRunCmd(), newHookGuardCmd())
	return cmd
}

func newHookPathsCmd() *cobra.Command {
	var target string
	var withAction, asJSON bool
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Print the files an edit hook's payload touched, one per line",
		Long: "Reads a hook payload on stdin and prints the files the edit leaves on disk, " +
			"one per line, relative to the current directory. The target comes from --target " +
			"or AGNOSTIC_AI_TARGET; with neither, a Claude Code file tool payload reads as claude. " +
			"Reads " + strings.Join(hookpaths.Targets(), ", ") + ". " +
			"A tool call that is no edit prints nothing. Invalid JSON, an edit tool input that is " +
			"no object, and a missing or unsupported target exit 1.",
		Example: `  # Format the Go files the agent edited; a failure stops the hook
  files=$(agnostic-ai hook paths) || exit 1
  printf '%s\n' "$files" | grep '\.go$' | while IFS= read -r f; do gofmt -w "$f"; done

  # Deleted files and move sources too, with the action in front
  agnostic-ai hook paths --action`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if withAction && asJSON {
				return errors.New("--action and --json are mutually exclusive")
			}
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			if target == "" {
				target = os.Getenv(adapters.HookTargetEnv)
			}
			if target == "" {
				target = hookpaths.GuessTarget(raw)
			}
			if target == "" {
				return fmt.Errorf("no target: pass --target or set %s", adapters.HookTargetEnv)
			}
			payload, err := hookpaths.Read(target, raw)
			if err != nil {
				return err
			}
			root, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			return printHookPaths(cmd.OutOrStdout(), payload.Relative(root), withAction, asJSON)
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target whose payload stdin holds (default $"+adapters.HookTargetEnv+")")
	cmd.Flags().BoolVar(&withAction, "action", false, "Print every change as <action><TAB><path>, deletes included")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print every change as a JSON array of {action, path, from}")
	_ = cmd.RegisterFlagCompletionFunc("target", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return hookpaths.Targets(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func printHookPaths(w io.Writer, changes []hookpaths.Change, withAction, asJSON bool) error {
	if asJSON {
		if changes == nil {
			changes = []hookpaths.Change{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(changes)
	}
	var b strings.Builder
	for _, c := range changes {
		switch {
		case withAction:
			b.WriteString(string(c.Action) + "\t" + c.Path + "\n")
		case c.Written():
			b.WriteString(c.Path + "\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
