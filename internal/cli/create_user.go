package cli

import (
	"fmt"
	"time"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
	"github.com/spf13/cobra"
)

func createUserCmd() *cobra.Command {
	var name string
	var natsURL string

	cmd := &cobra.Command{
		Use:   "create-user <email>",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]

			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			users := store.NewUserStore(s)
			userID := store.GenerateUserID()

			user := &mycelium.User{
				ID:        userID,
				Email:     email,
				Name:      name,
				CreatedAt: time.Now().UTC(),
			}

			if err := users.Create(user); err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			fmt.Printf("Created user: %s (%s) — %s\n", user.Name, user.Email, user.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "user display name")
	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")

	return cmd
}
