package provision

import (
	"fmt"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// LandProvisioner creates land instances on the land server.
type LandProvisioner struct {
	server string // SSH target
}

// NewLandProvisioner creates a new provisioner targeting the given server.
func NewLandProvisioner(server string) *LandProvisioner {
	return &LandProvisioner{server: server}
}

// Provision creates a land instance for the organization.
func (lp *LandProvisioner) Provision(org *mycelium.Organization) error {
	if org.LandPort == 0 {
		return fmt.Errorf("organization %s has no land port allocated", org.Slug)
	}

	// Phase 5: SSH-based provisioning
	// 1. Write /etc/land/<slug>/land.yaml
	// 2. Create land-<slug>.service
	// 3. systemctl enable --now land-<slug>

	return fmt.Errorf("SSH provisioning not yet implemented (use manual steps)")
}
