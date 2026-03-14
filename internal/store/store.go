// Package store provides a thin KV wrapper over NATS JetStream for mycelium data.
package store

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

const bucketName = "MYCELIUM_SOIL"

// Store wraps a NATS JetStream KV bucket for mycelium identity data.
type Store struct {
	kv nats.KeyValue
}

// New creates a new Store backed by a JetStream KV bucket.
func New(js nats.JetStreamContext) (*Store, error) {
	kv, err := js.KeyValue(bucketName)
	if err != nil {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      bucketName,
			Description: "Mycelium identity data",
			History:     10,
			Storage:     nats.FileStorage,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create KV bucket %s: %w", bucketName, err)
		}
		log.Printf("[Store] Created KV bucket: %s", bucketName)
	} else {
		log.Printf("[Store] Using existing KV bucket: %s", bucketName)
	}

	return &Store{kv: kv}, nil
}

// Get retrieves a value by key.
func (s *Store) Get(key string) ([]byte, error) {
	entry, err := s.kv.Get(key)
	if err != nil {
		if err == nats.ErrKeyNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get %s: %w", key, err)
	}
	return entry.Value(), nil
}

// Put stores a value by key (unconditional write).
func (s *Store) Put(key string, data []byte) error {
	_, err := s.kv.Put(key, data)
	if err != nil {
		return fmt.Errorf("failed to put %s: %w", key, err)
	}
	return nil
}

// Delete removes a key.
func (s *Store) Delete(key string) error {
	return s.kv.Delete(key)
}

// Keys returns all keys matching a pattern.
func (s *Store) Keys() ([]string, error) {
	keys, err := s.kv.Keys()
	if err != nil {
		if err == nats.ErrNoKeysFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	return keys, nil
}

// PutJSON marshals v to JSON and stores it.
func (s *Store) PutJSON(key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	return s.Put(key, data)
}

// GetJSON retrieves a value and unmarshals it.
func (s *Store) GetJSON(key string, v any) error {
	data, err := s.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = fmt.Errorf("not found")
