package cli

import (
	"fmt"
	"time"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
	"github.com/spf13/cobra"
)

func grantPassportCmd() *cobra.Command {
	var role string
	var natsURL string

	cmd := &cobra.Command{
		Use:   "grant-passport <user_id> <organization_slug>",
		Short: "Grant a user membership in an organization",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			userID := args[0]
			orgSlug := args[1]

			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			// Verify user exists
			users := store.NewUserStore(s)
			user, err := users.Get(userID)
			if err != nil {
				return fmt.Errorf("user %s not found", userID)
			}

			// Verify organization exists
			organizations := store.NewOrganizationStore(s)
			org, err := organizations.Get(orgSlug)
			if err != nil {
				return fmt.Errorf("organization %s not found", orgSlug)
			}

			// Create membership
			memberships := store.NewMembershipStore(s)
			m := &mycelium.Membership{
				UserID:           userID,
				OrganizationSlug: orgSlug,
				Role:             role,
				JoinedAt:         time.Now().UTC(),
			}

			if err := memberships.Grant(m); err != nil {
				return fmt.Errorf("failed to grant passport: %w", err)
			}

			fmt.Printf("Granted %s access to %s (%s) as %s\n", user.Name, org.Name, org.Slug, role)
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "member", "membership role (admin, member, agent)")
	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")

	return cmd
}
