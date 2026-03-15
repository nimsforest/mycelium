package identity

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nimsforest/mycelium/internal/store"
	"github.com/nimsforest/mycelium/pkg/mycelium"
)

// Consumer subscribes to the HUMUS JetStream stream and writes
// platform link entries to MYCELIUM_SOIL for eventual consistency.
type Consumer struct {
	js    nats.JetStreamContext
	store *store.Store
	sub   *nats.Subscription
}

// compostEntry mirrors the Compost struct from nimsforest2/internal/core/humus.go.
type compostEntry struct {
	Entity string          `json:"entity"`
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
}

// NewConsumer creates a new Humus consumer.
func NewConsumer(js nats.JetStreamContext, s *store.Store) *Consumer {
	return &Consumer{js: js, store: s}
}

// Start subscribes to the HUMUS stream with a durable consumer.
func (c *Consumer) Start() error {
	sub, err := c.js.Subscribe("humus.>", c.handle,
		nats.Durable("mycelium-identity"),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("failed to subscribe to HUMUS: %w", err)
	}
	c.sub = sub
	log.Printf("[Consumer] Listening on HUMUS stream (consumer: mycelium-identity)")
	return nil
}

// Stop unsubscribes the consumer.
func (c *Consumer) Stop() {
	if c.sub != nil {
		c.sub.Unsubscribe()
	}
}

func (c *Consumer) handle(msg *nats.Msg) {
	var entry compostEntry
	if err := json.Unmarshal(msg.Data, &entry); err != nil {
		log.Printf("[Consumer] Failed to unmarshal compost: %v", err)
		msg.Nak()
		return
	}

	// Only process platform link entries
	if !strings.HasPrefix(entry.Entity, "platforms.") {
		msg.Ack()
		return
	}

	switch entry.Action {
	case "create", "update":
		var link mycelium.PlatformLink
		if err := json.Unmarshal(entry.Data, &link); err != nil {
			log.Printf("[Consumer] Failed to unmarshal platform link data: %v", err)
			msg.Nak()
			return
		}
		if err := c.store.PutJSON(entry.Entity, &link); err != nil {
			log.Printf("[Consumer] Failed to write %s to MYCELIUM_SOIL: %v", entry.Entity, err)
			msg.Nak()
			return
		}
		log.Printf("[Consumer] Wrote %s to MYCELIUM_SOIL (user: %s)", entry.Entity, link.UserID)

	case "delete":
		if err := c.store.Delete(entry.Entity); err != nil {
			log.Printf("[Consumer] Failed to delete %s from MYCELIUM_SOIL: %v", entry.Entity, err)
		}
		log.Printf("[Consumer] Deleted %s from MYCELIUM_SOIL", entry.Entity)
	}

	msg.Ack()
}
