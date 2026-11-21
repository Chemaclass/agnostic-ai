package cli

import (
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newListCmd() *cobra.Command {
	var global, local bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.NoArgs,
		Short: "List loaded specs.",
		Example: `  # Print every loaded entry as <kind>\t<name>\t<layer>
  agnostic-ai list

  # Print effective global specs and their layers
  agnostic-ai list --scope user

  # Print only your personal specs in this project, and what each changes
  agnostic-ai list --scope local`,
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
			if local {
				entries = slices.DeleteFunc(slices.Clone(entries), func(e spec.Entry) bool { return e.Layer != layerNameProjectUser })
			}
			if len(entries) == 0 {
				if !jsonOut {
					hint := scope.emptyHint()
					if local {
						hint = "no local specs in " + defaultProjectUser + "/. add one with `agnostic-ai new <kind> <name> --scope local`."
					}
					cmd.PrintErrln(hint)
					return nil
				}
			}
			if jsonOut {
				refs := make([]listedSpec, 0, len(entries))
				for _, e := range entries {
					refs = append(refs, listedSpec{entrySourceRef(e), entryOverrides(e)})
				}
				return writeIndentedJSON(cmd, struct {
					Version string       `json:"version"`
					Command string       `json:"command"`
					Entries []listedSpec `json:"entries"`
				}{"1", "list", refs})
			}
			for _, e := range entries {
				layer := e.Layer
				if layer == "" {
					layer = layerNameProject
				}
				switch origin, base := builtinForEntry(e), entryOverrides(e); {
				case origin != nil:
					cmd.Printf("%s\t%s\t%s\t%s\n", e.Kind, e.Name, layer, origin.String())
				case base != nil:
					cmd.Printf("%s\t%s\t%s\toverrides %s\n", e.Kind, e.Name, layer, base.Layer)
				default:
					cmd.Printf("%s\t%s\t%s\n", e.Kind, e.Name, layer)
				}
			}
			return nil
		},
	}
	addScopeFlagWithLocal(cmd, &global, &local, "local lists only your personal specs in this project; user lists the specs of $AGNOSTIC_AI_HOME (default ~/.agnostic-ai) and their layers.")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output specs and their provenance as JSON")
	return cmd
}

// emptySpecsHint is shown by list/validate when no entries are loaded so a
// new user sees an actionable next step instead of silence.
const emptySpecsHint = "no specs found. add files under your sources " +
	"(default: .agnostic-ai/{agents,skills,rules,hooks,mcps}/) " +
	"or run `agnostic-ai import <source>`."
