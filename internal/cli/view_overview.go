package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func newViewCmd() *cobra.Command {
	return newViewCmdWithBrowser(openBrowser)
}

func newViewCmdWithBrowser(browser func(string) error) *cobra.Command {
	var dir, addr string
	var asJSON, global bool
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Open your configuration in the local web interface.",
		Long: "Opens the bundled web interface for the project in the current directory " +
			"or a parent directory, with a switch to the user-level configuration. Outside a " +
			"project, reads the user-level configuration. " +
			"Use --json to print the overview/v1 document without starting a server. " +
			"The interface reloads configuration on every request. Stops on Ctrl-C.",
		Example: `  # Open the project, or user configuration outside a project
  agnostic-ai view

  # Print the overview document of another project
  agnostic-ai view --dir ../service --json

  # Open the user-level configuration
  agnostic-ai view --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			root, useGlobal, err := resolveViewSource(dir, global, cmd.Flags().Changed("dir"))
			if err != nil {
				return err
			}
			if asJSON {
				in, err := loadOverviewInputs(root, useGlobal)
				if err != nil {
					return err
				}
				return writeIndentedJSON(cmd, overview.Build(in))
			}
			src, err := openAuthoritySource(root, useGlobal)
			if err != nil {
				return err
			}
			_, _, err = src.load()
			src.close()
			if err != nil {
				return err
			}
			serve := newServeCmdWithBrowser(browser)
			serve.SetContext(cmd.Context())
			serve.SetOut(cmd.OutOrStdout())
			serve.SetErr(cmd.ErrOrStderr())
			args := []string{"--open", "--addr", addr}
			if useGlobal {
				args = append(args, "--global")
			} else {
				args = append(args, "--dir", root)
			}
			serve.SetArgs(args)
			return serve.Execute()
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().StringVar(&addr, "addr", defaultServeAddr, "Loopback address to listen on.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the overview as JSON without opening the web interface.")
	cmd.Flags().BoolVar(&global, "global", false, "Read the user-level configuration in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	return cmd
}

func resolveViewSource(dir string, global, explicitDir bool) (string, bool, error) {
	if global {
		return "", true, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", false, fmt.Errorf("resolve project directory %s: %w", dir, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", false, fmt.Errorf("project directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("project directory %s is not a directory", root)
	}
	if explicitDir {
		return root, false, nil
	}
	root = canonicalDir(root)
	var globalRoots []os.FileInfo
	if source, err := globalSourceRoot(); err == nil {
		for _, path := range globalConfigPaths(source) {
			if info, err := os.Stat(filepath.Dir(path)); err == nil {
				globalRoots = append(globalRoots, info)
			}
		}
	}
	for candidate := root; ; candidate = filepath.Dir(candidate) {
		for _, globalRoot := range globalRoots {
			if info, err := os.Stat(candidate); err == nil && os.SameFile(info, globalRoot) {
				return "", true, nil
			}
		}
		for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName} {
			path := filepath.Join(candidate, name)
			if _, err := os.Lstat(path); err == nil {
				return candidate, false, nil
			} else if !os.IsNotExist(err) {
				return "", false, fmt.Errorf("inspect %s: %w", path, err)
			}
		}
		if filepath.Dir(candidate) == candidate {
			return "", true, nil
		}
	}
}

func loadOverviewInputs(dir string, global bool) (overview.Inputs, error) {
	if global {
		return globalOverviewInputs()
	}
	src, err := openAuthoritySource(dir, false)
	if err != nil {
		return overview.Inputs{}, err
	}
	defer src.close()
	return overviewInputs(".")
}
