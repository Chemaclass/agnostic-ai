package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			for _, e := range b.All() {
				layer := e.Layer
				if layer == "" {
					layer = layerNameProject
				}
				fmt.Printf("%s\t%s\t%s\n", e.Kind, e.Name, layer)
			}
			return nil
		},
	}
}
