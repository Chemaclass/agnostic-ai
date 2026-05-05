package cli

import (
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		Example: `  # Print every loaded entry as <kind>\t<name>\t<layer>
  agnostic-ai list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			entries := b.All()
			if len(entries) == 0 {
				cmd.PrintErrln(emptySpecsHint)
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
}

// emptySpecsHint is shown by list/validate when no entries are loaded so a
// new user sees an actionable next step instead of silence.
const emptySpecsHint = "no specs found. add files under your sources " +
	"(default: .agnostic-ai/{agents,skills,rules,hooks,mcps}/) " +
	"or run `agnostic-ai import <source>`."
