package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			fmt.Printf("loaded %d entries. ok.\n", len(b.All()))
			return nil
		},
	}
}
