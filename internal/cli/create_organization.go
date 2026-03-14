package cli

import (
	"fmt"
	"time"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
	"github.com/spf13/cobra"
)

func createOrganizationCmd() *cobra.Command {
	var slug string
	var natsURL string

	cmd := &cobra.Command{
		Use:   "create-organization <name>",
		Short: "Create a new organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if slug == "" {
				return fmt.Errorf("--slug is required")
			}

			nc, s, err := connectAndStore(natsURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			organizations := store.NewOrganizationStore(s)

			org := &mycelium.Organization{
				Slug:      slug,
				Name:      name,
				CreatedAt: time.Now().UTC(),
			}

			if err := organizations.Create(org); err != nil {
				return fmt.Errorf("failed to create organization: %w", err)
			}

			fmt.Printf("Created organization: %s (%s)\n", org.Name, org.Slug)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "organization slug (required)")
	cmd.Flags().StringVar(&natsURL, "nats", "nats://127.0.0.1:4222", "NATS server URL")
	cmd.MarkFlagRequired("slug")

	return cmd
}
