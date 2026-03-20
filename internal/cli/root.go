// Package cli defines the cobra command tree for the gw CLI.
package cli

import (
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"

	// Commit is set at build time via ldflags.
	Commit = "none"
)

var rootCmd = &cobra.Command{
	Use:   "gw",
	Short: "Ghostwriter CLI",
	Long: `Ghostwriter teaches AI coding agents to write in your personal voice and style.

The gw CLI handles the full workflow: collecting your writing samples,
embedding them into a vector database, and installing style instructions
for your AI tools.`,
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(
		newInitCmd(),
		newCollectCmd(),
		newIngestCmd(),
		newInstallCmd(),
		newQdrantCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)
}
