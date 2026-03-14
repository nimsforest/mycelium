package store

import (
	"strings"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// PassportStore manages passport records.
type PassportStore struct {
	s *Store
}

// NewPassportStore creates a new PassportStore.
func NewPassportStore(s *Store) *PassportStore {
	return &PassportStore{s: s}
}

func passportKey(agentID string) string { return "passports." + agentID }

// Create stores a new passport.
func (ps *PassportStore) Create(p *mycelium.Passport) error {
	return ps.s.PutJSON(passportKey(p.AgentID), p)
}

// Get retrieves a passport by agent ID.
func (ps *PassportStore) Get(agentID string) (*mycelium.Passport, error) {
	var p mycelium.Passport
	if err := ps.s.GetJSON(passportKey(agentID), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns all passports.
func (ps *PassportStore) List() ([]*mycelium.Passport, error) {
	keys, err := ps.s.Keys()
	if err != nil {
		return nil, err
	}
	var passports []*mycelium.Passport
	for _, k := range keys {
		if strings.HasPrefix(k, "passports.") {
			p, err := ps.Get(strings.TrimPrefix(k, "passports."))
			if err != nil {
				continue
			}
			passports = append(passports, p)
		}
	}
	return passports, nil
}
