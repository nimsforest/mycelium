package cli

import (
	"fmt"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/spf13/cobra"
)

func listOrganizationsCmd() *cobra.Command {
	var natsURL string

	cmd := &cobra.Command{
		Use:   "list-organizations",
		Short: "List all organizations",
		RunE: func(cmd *cobra.Command, args []string) error {
			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			organizations := store.NewOrganizationStore(s)
			list, err := organizations.List()
			if err != nil {
				return err
			}

			if len(list) == 0 {
				fmt.Println("No organizations found.")
				return nil
			}

			fmt.Printf("%-20s %-30s %-10s %-10s %s\n", "SLUG", "NAME", "NATS", "LAND", "CREATED")
			for _, org := range list {
				natsPort := "-"
				landPort := "-"
				if org.NATSPort > 0 {
					natsPort = fmt.Sprintf(":%d", org.NATSPort)
				}
				if org.LandPort > 0 {
					landPort = fmt.Sprintf(":%d", org.LandPort)
				}
				fmt.Printf("%-20s %-30s %-10s %-10s %s\n",
					org.Slug, org.Name, natsPort, landPort,
					org.CreatedAt.Format("2006-01-02"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")

	return cmd
}

func listUsersCmd() *cobra.Command {
	var natsURL string

	cmd := &cobra.Command{
		Use:   "list-users",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			users := store.NewUserStore(s)
			list, err := users.List()
			if err != nil {
				return err
			}

			if len(list) == 0 {
				fmt.Println("No users found.")
				return nil
			}

			memberships := store.NewMembershipStore(s)

			fmt.Printf("%-16s %-20s %-30s %s\n", "ID", "NAME", "EMAIL", "ORGANIZATIONS")
			for _, u := range list {
				organizations, _ := memberships.GetUserOrganizations(u.ID)
				orgStr := "-"
				if len(organizations) > 0 {
					orgStr = fmt.Sprintf("%v", organizations)
				}
				fmt.Printf("%-16s %-20s %-30s %s\n", u.ID, u.Name, u.Email, orgStr)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")

	return cmd
}
