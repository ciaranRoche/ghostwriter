package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the gw version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gw %s (commit: %s)\n", Version, Commit)
		},
	}
}
