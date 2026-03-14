// Package provision handles creating forest and land processes for new organizations.
package provision

import (
	"fmt"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// ForestProvisioner creates forest instances on the nimsforest server.
type ForestProvisioner struct {
	server string // SSH target
}

// NewForestProvisioner creates a new provisioner targeting the given server.
func NewForestProvisioner(server string) *ForestProvisioner {
	return &ForestProvisioner{server: server}
}

// Provision creates a forest instance for the organization.
// This includes creating the JetStream data directory, writing configs,
// creating a systemd service, and starting it.
func (fp *ForestProvisioner) Provision(org *mycelium.Organization) error {
	if org.NATSPort == 0 {
		return fmt.Errorf("organization %s has no NATS port allocated", org.Slug)
	}

	// Phase 5: SSH-based provisioning
	// 1. mkdir -p /var/lib/nimsforest/<slug>/jetstream
	// 2. Write /etc/nimsforest/<slug>/forest.yaml
	// 3. Write /etc/nimsforest/<slug>/node-info.json
	// 4. Create forest-<slug>.service
	// 5. systemctl enable --now forest-<slug>
	// 6. forest seed (within the new instance)

	return fmt.Errorf("SSH provisioning not yet implemented (use manual steps)")
}
