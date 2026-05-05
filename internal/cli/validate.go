package cli

import (
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		Example: `  # Parse every spec and print the loaded count
  agnostic-ai validate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			n := len(b.All())
			cmd.Printf("loaded %d entries. ok.\n", n)
			if n == 0 {
				cmd.PrintErrln(emptySpecsHint)
			}
			return nil
		},
	}
}
