package cli

import (
	"fmt"
	"os"

	"github.com/cederikdotcom/hydrarelease/pkg/updater"
	"github.com/spf13/cobra"
)

// Root creates the root command with all subcommands.
func Root(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "mycelium",
		Short:         "Mycelium — central identity service for NimsForest",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(versionCmd(version))
	root.AddCommand(serveCmd(version))
	root.AddCommand(createOrganizationCmd())
	root.AddCommand(createUserCmd())
	root.AddCommand(linkPlatformCmd())
	root.AddCommand(grantPassportCmd())
	root.AddCommand(provisionCmd())
	root.AddCommand(listOrganizationsCmd())
	root.AddCommand(listUsersCmd())
	root.AddCommand(updateCmd(version))
	root.AddCommand(checkUpdateCmd(version))

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

func updateCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update mycelium to the latest release",
		RunE: func(cmd *cobra.Command, args []string) error {
			u := updater.NewProductionUpdater("mycelium", version)
			u.SetServiceName("mycelium")

			fmt.Println("Checking for updates...")
			info, err := u.CheckForUpdate()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to check for updates: %s\n", err)
				return err
			}

			fmt.Printf("Current version: %s\n", info.CurrentVersion)
			fmt.Printf("Latest version:  %s\n", info.LatestVersion)

			if !info.Available {
				fmt.Println("\nAlready running the latest version!")
				return nil
			}

			fmt.Println("\nA new version is available!")
			fmt.Print("\nUpdate now? (yes/no): ")
			var response string
			fmt.Scanln(&response)

			if response != "yes" && response != "y" {
				fmt.Println("Update cancelled.")
				return nil
			}

			fmt.Println()
			return u.PerformUpdate()
		},
	}
}

func checkUpdateCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "check-update",
		Short: "Check if a new version is available",
		RunE: func(cmd *cobra.Command, args []string) error {
			u := updater.NewProductionUpdater("mycelium", version)

			info, err := u.CheckForUpdate()
			if err != nil {
				return err
			}

			if info.Available {
				fmt.Printf("Update available: %s -> %s\n", info.CurrentVersion, info.LatestVersion)
				fmt.Println("Run 'mycelium update' to install.")
			} else {
				fmt.Printf("Already up to date: %s\n", info.CurrentVersion)
			}
			return nil
		},
	}
}
