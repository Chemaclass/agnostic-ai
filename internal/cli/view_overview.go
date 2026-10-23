package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// newViewOverviewCmd is `view overview`: the harness overview of a
// project, as text or as the overview/v1 JSON document.
func newViewOverviewCmd() *cobra.Command {
	var dir string
	var asJSON, global bool
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Show the harness overview: what each section holds, which tools support it, and what the checks report.",
		Long: "Prints the harness sections (context, authority, tools, lifecycle, " +
			"settings) with the items each holds, each item's lint and read-only " +
			"doctor findings, and whether each target's generated files match what " +
			"sync would write. Nothing is written. --json prints the overview/v1 " +
			"document the UI reads.",
		Example: `  # Summarize the project in the working directory
  agnostic-ai view overview

  # Print the overview document of another project
  agnostic-ai view overview --dir ../service --json

  # Summarize the user-level configuration
  agnostic-ai view overview --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			in, err := loadOverviewInputs(dir, global)
			if err != nil {
				return err
			}
			view := overview.Build(in)
			if asJSON {
				return writeIndentedJSON(cmd, view)
			}
			printOverview(cmd, view)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to inspect.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the overview as JSON on stdout.")
	cmd.Flags().BoolVar(&global, "global", false, "Read the user-level configuration sync --global uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	return cmd
}

// loadOverviewInputs builds the overview inputs of the project in dir,
// or of the user-level configuration when global is set.
func loadOverviewInputs(dir string, global bool) (overview.Inputs, error) {
	if global {
		return globalOverviewInputs()
	}
	leave, err := overviewEnterDir(dir)
	if err != nil {
		return overview.Inputs{}, err
	}
	defer leave()
	return overviewInputs(".")
}

// overviewEnterDir changes into dir for the command and returns the
// function that changes back. The checks resolve generated paths against
// the working directory, so they run inside the project.
func overviewEnterDir(dir string) (leave func(), err error) {
	if dir == "" || filepath.Clean(dir) == "." {
		return func() {}, nil
	}
	origin, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("--dir: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("--dir: %w", err)
	}
	return func() { _ = os.Chdir(origin) }, nil
}

// printOverview prints each target's sync state, then each section's
// counts and the findings on its items.
func printOverview(cmd *cobra.Command, v *overview.View) {
	cmd.Printf("Harness overview of %s (revision %s)\n", v.Snapshot.Project.Name, v.Snapshot.Revision)
	for _, t := range v.Targets {
		cmd.Printf("  %s: %s\n", t.Label, syncWording(t.Sync))
	}
	for _, s := range v.Sections {
		counts := make([]string, 0, len(s.Kinds))
		for _, k := range s.Kinds {
			counts = append(counts, fmt.Sprintf("%s %d", k.Label, k.Count))
		}
		cmd.Printf("\n%s — %s\n  %s\n", s.Label, s.Question, strings.Join(counts, " · "))
		for _, it := range s.Items {
			for _, f := range it.Findings {
				cmd.Printf("  ! %s: %s\n", it.Label, findingLine(f))
			}
		}
		for _, f := range s.Findings {
			cmd.Printf("  ! %s\n", findingLine(f))
		}
	}
}

func syncWording(s overview.TargetSync) string {
	switch s.State {
	case overview.SyncInSync:
		return "in sync"
	case overview.SyncDrifted:
		return fmt.Sprintf("out of sync (%d file(s))", len(s.Files))
	}
	return "not checked"
}

func findingLine(f overview.ItemFinding) string {
	if f.Code != "" {
		return fmt.Sprintf("%s [%s] %s", f.Code, f.Severity, f.Message)
	}
	return fmt.Sprintf("[%s] %s", f.Severity, f.Message)
}
