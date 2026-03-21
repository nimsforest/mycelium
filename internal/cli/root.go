package cli

import (
	"github.com/spf13/cobra"
)

// Root creates the root command with all subcommands.
func Root(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mycelium",
		Short:         "Mycelium — NATS auth service for NimsForest",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(versionCmd(version))
	root.AddCommand(serveCmd(version))

	return root
}

func versionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("mycelium", version)
		},
	}
}
