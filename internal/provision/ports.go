package provision

// AllocatePorts calculates the next available NATS and land ports
// based on the number of existing organizations.
func AllocatePorts(baseNATSPort, baseLandPort, organizationCount int) (natsPort, landPort int) {
	return baseNATSPort + organizationCount, baseLandPort + organizationCount
}
