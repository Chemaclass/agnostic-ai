package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var global bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		Example: `  # Print every loaded entry as <kind>\t<name>\t<layer>
  agnostic-ai list

  # Print effective global specs and their layers
  agnostic-ai list --global`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// list reads no home targets, so a home config that does not
			// parse only warns.
			warn := cmd.ErrOrStderr()
			if verbosity < levelDefault {
				warn = io.Discard
			}
			scope, err := loadSpecScope(global, warn)
			if err != nil {
				return err
			}
			entries := scope.bundle.All()
			if len(entries) == 0 {
				if !jsonOut {
					cmd.PrintErrln(scope.emptyHint())
					return nil
				}
			}
			if jsonOut {
				refs := make([]specSourceRef, 0, len(entries))
				for _, e := range entries {
					refs = append(refs, entrySourceRef(e))
				}
				return writeIndentedJSON(cmd, struct {
					Version string          `json:"version"`
					Command string          `json:"command"`
					Entries []specSourceRef `json:"entries"`
				}{"1", "list", refs})
			}
			for _, e := range entries {
				layer := e.Layer
				if layer == "" {
					layer = layerNameProject
				}
				if origin := builtinForEntry(e); origin != nil {
					cmd.Printf("%s\t%s\t%s\t%s\n", e.Kind, e.Name, layer, origin.String())
				} else {
					cmd.Printf("%s\t%s\t%s\n", e.Kind, e.Name, layer)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "List effective global specs and their layers")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output specs and their provenance as JSON")
	return cmd
}

// emptySpecsHint is shown by list/validate when no entries are loaded so a
// new user sees an actionable next step instead of silence.
const emptySpecsHint = "no specs found. add files under your sources " +
	"(default: .agnostic-ai/{agents,skills,rules,hooks,mcps}/) " +
	"or run `agnostic-ai import <source>`."
