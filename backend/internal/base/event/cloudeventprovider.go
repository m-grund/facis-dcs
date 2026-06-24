package event

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/cloudevents/sdk-go/v2/event"
	cloudeventprovider "github.com/eclipse-xfsc/cloud-event-provider"
)

type CloudEventPubClient struct {
	ctx    context.Context
	cancel context.CancelFunc
	topic  string
	client *cloudeventprovider.CloudEventProviderClient
}

func (c CloudEventPubClient) Close() error {
	c.cancel()
	return c.client.Close()
}

func (c CloudEventPubClient) Publish(eventSource string, eventType string, payload json.RawMessage) interface{} {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}
	evt, err := cloudeventprovider.NewEvent(eventSource, eventType, data)
	if err != nil {
		log.Fatal(err)
	}

	return c.client.PubCtx(c.ctx, evt)
}

func NewNatsPubClient(topic string, natsURL string) (*CloudEventPubClient, error) {
	client, err := cloudeventprovider.New(cloudeventprovider.Config{
		Protocol: cloudeventprovider.ProtocolTypeNats,
		Settings: cloudeventprovider.NatsConfig{
			Url: natsURL,
		},
	}, cloudeventprovider.ConnectionTypePub, topic)
	if err != nil {
		return nil, errors.New("could not create cloud event provider client")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &CloudEventPubClient{ctx, cancel, topic, client}, nil
}

type CloudEventSubClient struct {
	ctx    context.Context
	cancel context.CancelFunc
	topic  string
	client *cloudeventprovider.CloudEventProviderClient
}

func (c CloudEventSubClient) Close() error {
	c.cancel()
	return c.client.Close()
}

func (c CloudEventSubClient) Cancel() {
	c.cancel()
}

func (c CloudEventSubClient) Subscribe(f func(evt event.Event)) error {
	return c.client.SubCtx(c.ctx, f)
}

func NewNatsSubClient(topic string, natsURL string) (*CloudEventSubClient, error) {
	client, err := cloudeventprovider.New(cloudeventprovider.Config{
		Protocol: cloudeventprovider.ProtocolTypeNats,
		Settings: cloudeventprovider.NatsConfig{
			Url: natsURL,
		},
	}, cloudeventprovider.ConnectionTypeSub, topic)
	if err != nil {
		return nil, errors.New("could not create cloud event provider client")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &CloudEventSubClient{ctx, cancel, topic, client}, nil
}
