package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// newViewOverviewCmd is `view overview`: the harness overview of a
// project, as text or as the overview/v1 JSON document.
func newViewOverviewCmd() *cobra.Command {
	var dir string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Show the harness overview: what each section holds, which tools support it, and a feed of findings.",
		Long: "Prints the six harness sections (context, memory, control, authority, " +
			"tools, verification) with the specs each holds, then the feed: cards " +
			"derived from what lint, the read-only doctor checks, and a comparison of " +
			"generated files with disk report. Nothing is written. --json prints the " +
			"overview/v1 document the UI reads.",
		Example: `  # Summarize the project in the working directory
  agnostic-ai view overview

  # Print the overview document of another project
  agnostic-ai view overview --dir ../service --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			leave, err := overviewEnterDir(dir)
			if err != nil {
				return err
			}
			defer leave()
			in, err := overviewInputs(".", nil)
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
	return cmd
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

// printOverview prints each section's counts, then the feed's titles.
func printOverview(cmd *cobra.Command, v *overview.View) {
	cmd.Printf("Harness overview of %s (revision %s)\n", v.Snapshot.Project.Name, v.Snapshot.Revision)
	if len(v.Snapshot.Targets) > 0 {
		cmd.Printf("Targets: %s\n", strings.Join(v.Snapshot.Targets, ", "))
	}
	for _, s := range v.Sections {
		counts := make([]string, 0, len(s.Kinds))
		for _, k := range s.Kinds {
			counts = append(counts, fmt.Sprintf("%s %d", k.Label, k.Count))
		}
		cmd.Printf("\n%s — %s\n  %s\n", s.Label, s.Question, strings.Join(counts, " · "))
	}
	cmd.Println()
	if len(v.Feed) == 0 {
		cmd.Println("Feed: no findings.")
		return
	}
	cmd.Printf("Feed: %d card(s)\n", len(v.Feed))
	for _, c := range v.Feed {
		cmd.Printf("  %-11s %s\n", c.PriorityLabel, c.Title)
	}
}
