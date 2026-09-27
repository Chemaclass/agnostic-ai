package cli

import (
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		Example: `  # Print every loaded entry as <kind>\t<name>\t<layer>
  agnostic-ai list

  # Print effective global specs and their layers
  agnostic-ai list --global`,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := loadCheckScope(global)
			if err != nil {
				return err
			}
			entries := scope.bundle.All()
			if len(entries) == 0 {
				cmd.PrintErrln(scope.emptyHint())
				return nil
			}
			for _, e := range entries {
				layer := e.Layer
				if layer == "" {
					layer = layerNameProject
				}
				cmd.Printf("%s\t%s\t%s\n", e.Kind, e.Name, layer)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "List effective global specs and their layers")
	return cmd
}

// emptySpecsHint is shown by list/validate when no entries are loaded so a
// new user sees an actionable next step instead of silence.
const emptySpecsHint = "no specs found. add files under your sources " +
	"(default: .agnostic-ai/{agents,skills,rules,hooks,mcps}/) " +
	"or run `agnostic-ai import <source>`."
