package cli

import (
	"fmt"

	"github.com/nimsforest/mycelium/internal/store"
	"github.com/spf13/cobra"
)

func provisionCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "provision <organization_slug>",
		Short: "Provision forest and land processes for an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]

			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}

			nc, s, err := connectAndStore(cfg.NATSURL)
			if err != nil {
				return err
			}
			defer nc.Close()

			// Get organization
			organizations := store.NewOrganizationStore(s)
			org, err := organizations.Get(slug)
			if err != nil {
				return fmt.Errorf("organization %s not found", slug)
			}

			// Allocate ports
			allOrganizations, err := organizations.List()
			if err != nil {
				return fmt.Errorf("failed to list organizations: %w", err)
			}

			natsPort := cfg.BaseNATSPort + len(allOrganizations)
			landPort := cfg.BaseLandPort + len(allOrganizations)

			// Skip if already provisioned
			if org.NATSPort > 0 {
				return fmt.Errorf("organization %s is already provisioned (NATS port: %d)", slug, org.NATSPort)
			}

			fmt.Printf("Provisioning %s (%s)...\n", org.Name, org.Slug)
			fmt.Printf("  NATS port: %d\n", natsPort)
			fmt.Printf("  Land port: %d\n", landPort)
			fmt.Printf("  Forest server: %s\n", cfg.ForestServer)
			fmt.Printf("  Land server: %s\n", cfg.LandServer)

			// TODO: SSH provisioning (Phase 5)
			// For now, just update the organization record with allocated ports
			org.NATSPort = natsPort
			org.LandPort = landPort

			if err := organizations.Update(org); err != nil {
				return fmt.Errorf("failed to update organization: %w", err)
			}

			fmt.Printf("\nPorts allocated for %s. Manual provisioning steps:\n", org.Slug)
			fmt.Printf("  1. On forest server (%s):\n", cfg.ForestServer)
			fmt.Printf("     mkdir -p /var/lib/nimsforest/%s/jetstream\n", org.Slug)
			fmt.Printf("     # Write /etc/nimsforest/%s/forest.yaml\n", org.Slug)
			fmt.Printf("     # Create forest-%s.service\n", org.Slug)
			fmt.Printf("  2. On land server (%s):\n", cfg.LandServer)
			fmt.Printf("     # Write /etc/land/%s/land.yaml\n", org.Slug)
			fmt.Printf("     # Create land-%s.service\n", org.Slug)

			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "/etc/mycelium/mycelium.yaml", "path to config file")

	return cmd
}
