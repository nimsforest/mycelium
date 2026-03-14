package store

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// UserStore manages user records.
type UserStore struct {
	s *Store
}

// NewUserStore creates a new UserStore.
func NewUserStore(s *Store) *UserStore {
	return &UserStore{s: s}
}

func userKey(id string) string { return "users." + id }

// Create stores a new user.
func (us *UserStore) Create(u *mycelium.User) error {
	return us.s.PutJSON(userKey(u.ID), u)
}

// Get retrieves a user by ID.
func (us *UserStore) Get(id string) (*mycelium.User, error) {
	var u mycelium.User
	if err := us.s.GetJSON(userKey(id), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users.
func (us *UserStore) List() ([]*mycelium.User, error) {
	keys, err := us.s.Keys()
	if err != nil {
		return nil, err
	}
	var users []*mycelium.User
	for _, k := range keys {
		if strings.HasPrefix(k, "users.") {
			u, err := us.Get(strings.TrimPrefix(k, "users."))
			if err != nil {
				continue
			}
			users = append(users, u)
		}
	}
	return users, nil
}

// GenerateUserID creates a new user ID with the u_ prefix.
func GenerateUserID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("u_%x", b)
}
