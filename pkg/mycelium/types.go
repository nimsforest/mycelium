// Package mycelium provides shared types for the mycelium identity service.
package mycelium

import "time"

// User represents a human identity that spans organizations.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Organization represents an isolated forest instance.
type Organization struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	NATSPort  int       `json:"nats_port,omitempty"`
	LandPort  int       `json:"land_port,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Membership links a user to an organization with a role.
type Membership struct {
	UserID           string    `json:"user_id"`
	OrganizationSlug string    `json:"organization_slug"`
	Role             string    `json:"role"`
	JoinedAt         time.Time `json:"joined_at"`
}

// Passport grants an agent cross-organization access.
type Passport struct {
	AgentID              string   `json:"agent_id"`
	AgentType            string   `json:"agent_type"`
	HomeOrganization     string   `json:"home_organization"`
	AllowedOrganizations []string `json:"allowed_organizations"`
	Capabilities         []string `json:"capabilities"`
}

// PlatformLink maps a platform ID to a user.
type PlatformLink struct {
	UserID string `json:"user_id"`
}

// ResolveResponse is returned by the identity resolver.
type ResolveResponse struct {
	UserID        string   `json:"user_id"`
	Name          string   `json:"name"`
	Email         string   `json:"email,omitempty"`
	Organizations []string `json:"organizations,omitempty"`
	Role          string   `json:"role,omitempty"`
}

// PassportResponse is returned by passport resolution.
type PassportResponse struct {
	Allowed      bool     `json:"allowed"`
	AgentType    string   `json:"agent_type"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// PlatformEntry represents a single platform link for a user.
type PlatformEntry struct {
	Platform   string `json:"platform"`
	PlatformID string `json:"platform_id"`
}

// PlatformsResponse is returned by user platform resolution.
type PlatformsResponse struct {
	UserID    string          `json:"user_id"`
	Platforms []PlatformEntry `json:"platforms"`
}

// MemberList stores organization member IDs (reverse index).
type MemberList struct {
	UserIDs []string `json:"user_ids"`
}

// OrganizationList stores user organization slugs (reverse index).
type OrganizationList struct {
	OrganizationSlugs []string `json:"organization_slugs"`
}
