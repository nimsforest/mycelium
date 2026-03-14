package store

import (
	"strings"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// OrganizationStore manages organization records.
type OrganizationStore struct {
	s *Store
}

// NewOrganizationStore creates a new OrganizationStore.
func NewOrganizationStore(s *Store) *OrganizationStore {
	return &OrganizationStore{s: s}
}

func organizationKey(slug string) string { return "organizations." + slug }

// Create stores a new organization.
func (os *OrganizationStore) Create(o *mycelium.Organization) error {
	return os.s.PutJSON(organizationKey(o.Slug), o)
}

// Get retrieves an organization by slug.
func (os *OrganizationStore) Get(slug string) (*mycelium.Organization, error) {
	var o mycelium.Organization
	if err := os.s.GetJSON(organizationKey(slug), &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// Update stores an updated organization.
func (os *OrganizationStore) Update(o *mycelium.Organization) error {
	return os.s.PutJSON(organizationKey(o.Slug), o)
}

// List returns all organizations.
func (os *OrganizationStore) List() ([]*mycelium.Organization, error) {
	keys, err := os.s.Keys()
	if err != nil {
		return nil, err
	}
	var organizations []*mycelium.Organization
	for _, k := range keys {
		if strings.HasPrefix(k, "organizations.") {
			o, err := os.Get(strings.TrimPrefix(k, "organizations."))
			if err != nil {
				continue
			}
			organizations = append(organizations, o)
		}
	}
	return organizations, nil
}
