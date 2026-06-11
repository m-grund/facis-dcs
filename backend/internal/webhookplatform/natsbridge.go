package webhookplatform

import (
	"context"
	"encoding/json"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
)

func StartNATSBridge(subClient *event.CloudEventSubClient, d *Dispatcher) error {
	return subClient.Subscribe(func(evt cloudevent.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		handleNATSEvent(ctx, evt, d)
	})
}

func handleNATSEvent(ctx context.Context, evt cloudevent.Event, d *Dispatcher) {
	eventType := evt.Type()
	if _, known := DCSEventMap[eventType]; !known {
		return
	}

	// The outbox publisher double-encodes the payload: json.Marshal([]byte).
	// DataAs(&[]byte) reverses that automatically.
	var rawPayload []byte
	if err := evt.DataAs(&rawPayload); err != nil {
		log.Printf("webhookplatform: decode NATS payload for %s: %v", eventType, err)
		return
	}

	var envelope struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(rawPayload, &envelope); err != nil {
		log.Printf("webhookplatform: unmarshal DID from %s payload: %v", eventType, err)
		return
	}

	d.DispatchFromDCS(ctx, eventType, envelope.DID, json.RawMessage(rawPayload))
}
