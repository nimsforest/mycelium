package cli

import (
	"fmt"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
	"github.com/spf13/cobra"
)

func linkPlatformCmd() *cobra.Command {
	var natsURL string

	cmd := &cobra.Command{
		Use:   "link-platform <user_id> <platform> <platform_id>",
		Short: "Link a platform identity to a user",
		Long:  "Map a platform identity (e.g., Telegram user ID) to a mycelium user.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			userID := args[0]
			platform := args[1]
			platformID := args[2]

			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			// Verify user exists
			users := store.NewUserStore(s)
			if _, err := users.Get(userID); err != nil {
				return fmt.Errorf("user %s not found", userID)
			}

			// Store platform link
			link := mycelium.PlatformLink{UserID: userID}
			key := fmt.Sprintf("platforms.%s.%s", platform, platformID)
			if err := s.PutJSON(key, &link); err != nil {
				return fmt.Errorf("failed to link platform: %w", err)
			}

			fmt.Printf("Linked %s:%s -> %s\n", platform, platformID, userID)
			return nil
		},
	}

	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")

	return cmd
}
