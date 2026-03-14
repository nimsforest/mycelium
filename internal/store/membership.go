package store

import (
	"fmt"
	"strings"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// MembershipStore manages membership and reverse index records.
type MembershipStore struct {
	s *Store
}

// NewMembershipStore creates a new MembershipStore.
func NewMembershipStore(s *Store) *MembershipStore {
	return &MembershipStore{s: s}
}

func membershipKey(slug, userID string) string {
	return fmt.Sprintf("memberships.%s.%s", slug, userID)
}
func orgMembersKey(slug string) string    { return "organization_members." + slug }
func userOrganizationsKey(id string) string { return "user_organizations." + id }

// Grant creates a membership and updates reverse indexes.
func (ms *MembershipStore) Grant(m *mycelium.Membership) error {
	// Store the membership record
	if err := ms.s.PutJSON(membershipKey(m.OrganizationSlug, m.UserID), m); err != nil {
		return err
	}

	// Update organization_members reverse index
	var members mycelium.MemberList
	if err := ms.s.GetJSON(orgMembersKey(m.OrganizationSlug), &members); err != nil && err != ErrNotFound {
		return err
	}
	if !contains(members.UserIDs, m.UserID) {
		members.UserIDs = append(members.UserIDs, m.UserID)
		if err := ms.s.PutJSON(orgMembersKey(m.OrganizationSlug), &members); err != nil {
			return err
		}
	}

	// Update user_organizations reverse index
	var userOrganizations mycelium.OrganizationList
	if err := ms.s.GetJSON(userOrganizationsKey(m.UserID), &userOrganizations); err != nil && err != ErrNotFound {
		return err
	}
	if !contains(userOrganizations.OrganizationSlugs, m.OrganizationSlug) {
		userOrganizations.OrganizationSlugs = append(userOrganizations.OrganizationSlugs, m.OrganizationSlug)
		if err := ms.s.PutJSON(userOrganizationsKey(m.UserID), &userOrganizations); err != nil {
			return err
		}
	}

	return nil
}

// GetUserOrganizations returns organization slugs for a user.
func (ms *MembershipStore) GetUserOrganizations(userID string) ([]string, error) {
	var list mycelium.OrganizationList
	if err := ms.s.GetJSON(userOrganizationsKey(userID), &list); err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return list.OrganizationSlugs, nil
}

// GetMembership retrieves a specific membership.
func (ms *MembershipStore) GetMembership(slug, userID string) (*mycelium.Membership, error) {
	var m mycelium.Membership
	if err := ms.s.GetJSON(membershipKey(slug, userID), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// GetOrganizationMembers returns user IDs for an organization.
func (ms *MembershipStore) GetOrganizationMembers(slug string) ([]string, error) {
	var list mycelium.MemberList
	if err := ms.s.GetJSON(orgMembersKey(slug), &list); err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return list.UserIDs, nil
}

// ListAll returns all memberships.
func (ms *MembershipStore) ListAll() ([]*mycelium.Membership, error) {
	keys, err := ms.s.Keys()
	if err != nil {
		return nil, err
	}
	var memberships []*mycelium.Membership
	for _, k := range keys {
		if strings.HasPrefix(k, "memberships.") {
			var m mycelium.Membership
			if err := ms.s.GetJSON(k, &m); err != nil {
				continue
			}
			memberships = append(memberships, &m)
		}
	}
	return memberships, nil
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
