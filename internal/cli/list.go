package cli

import (
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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
			var b spec.Bundle
			var err error
			if global {
				var home string
				home, err = globalUserHome()
				if err == nil {
					b, err = spec.LoadLayered(globalLayers(globalSourceHome(home)))
				}
			} else {
				_, b, err = loadProject(".")
			}
			if err != nil {
				return err
			}
			entries := b.All()
			if len(entries) == 0 {
				if global {
					cmd.PrintErrln("no global specs found. add files under $AGNOSTIC_AI_HOME/{agents,skills,rules,hooks}/ or its local/ layer (default home: ~/.agnostic-ai).")
				} else {
					cmd.PrintErrln(emptySpecsHint)
				}
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
