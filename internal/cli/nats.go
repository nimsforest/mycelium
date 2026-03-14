package cli

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nimsforest/mycelium/internal/store"
)

// connectAndStore creates a NATS connection and initializes the store.
func connectAndStore(natsURL string) (*nats.Conn, *store.Store, error) {
	nc, err := nats.Connect(natsURL,
		nats.Name("mycelium"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	s, err := store.New(js)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to create store: %w", err)
	}

	return nc, s, nil
}
