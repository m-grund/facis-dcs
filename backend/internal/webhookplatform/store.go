package webhookplatform

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// KnownEvents is the list of subscribable DCS contract lifecycle events.
var KnownEvents = []EventInfo{
	{Name: "contract.created", Description: "A new contract was created"},
	{Name: "contract.submitted", Description: "A contract was submitted for review"},
	{Name: "contract.approved", Description: "A contract was approved"},
	{Name: "contract.rejected", Description: "A contract was rejected"},
	{Name: "contract.negotiated", Description: "A contract entered negotiation"},
	{Name: "contract.terminated", Description: "A contract was terminated"},
	{Name: "template.created", Description: "A new contract template was created"},
	{Name: "template.approved", Description: "A contract template was approved"},
}

// DCSEventMap maps internal NATS event types to webhook event names.
var DCSEventMap = map[string]string{
	"CREATE_CONTRACT":           "contract.created",
	"SUBMIT_CONTRACT":           "contract.submitted",
	"APPROVE_CONTRACT":          "contract.approved",
	"REJECT_CONTRACT":           "contract.rejected",
	"NEGOTIATE_CONTRACT":        "contract.negotiated",
	"TERMINATE_CONTRACT":        "contract.terminated",
	"CREATE_CONTRACT_TEMPLATE":  "template.created",
	"APPROVE_CONTRACT_TEMPLATE": "template.approved",
}

// EventInfo describes a subscribable event.
type EventInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Subscription represents a registered webhook.
type Subscription struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	CallbackURL string    `json:"callback_url"`
	Secret      string    `json:"secret,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PendingCallback tracks an in-flight webhook dispatch awaiting a callback.
type PendingCallback struct {
	EventID       string
	CorrelationID string
	Event         string
	DID           string
	SentAt        time.Time
}

// SubscriptionStore is a thread-safe in-memory store for subscriptions and
// pending callbacks.
type SubscriptionStore struct {
	mu      sync.RWMutex
	subs    map[string][]Subscription  // event name → subscriptions
	pending map[string]PendingCallback // event_id → pending
}

// NewSubscriptionStore returns an empty store.
func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		subs:    make(map[string][]Subscription),
		pending: make(map[string]PendingCallback),
	}
}

// Add registers a new webhook subscription and returns it with a generated ID.
func (s *SubscriptionStore) Add(event, callbackURL, secret string) Subscription {
	sub := Subscription{
		ID:          uuid.New().String(),
		Event:       event,
		CallbackURL: callbackURL,
		Secret:      secret,
		CreatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.subs[event] = append(s.subs[event], sub)
	s.mu.Unlock()
	return sub
}

// Delete removes a subscription by ID. Returns false when not found.
func (s *SubscriptionStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for event, subs := range s.subs {
		for i, sub := range subs {
			if sub.ID == id {
				s.subs[event] = append(subs[:i], subs[i+1:]...)
				return true
			}
		}
	}
	return false
}

// GetByEvent returns a snapshot of all subscriptions for the given event.
func (s *SubscriptionStore) GetByEvent(event string) []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Subscription(nil), s.subs[event]...)
}

// ListAll returns all registered subscriptions.
func (s *SubscriptionStore) ListAll() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []Subscription
	for _, subs := range s.subs {
		all = append(all, subs...)
	}
	return all
}

// TrackPending records an in-flight dispatch so its callback can be correlated.
func (s *SubscriptionStore) TrackPending(p PendingCallback) {
	s.mu.Lock()
	s.pending[p.EventID] = p
	s.mu.Unlock()
}

// ResolvePending looks up and removes a pending callback by event_id.
func (s *SubscriptionStore) ResolvePending(eventID string) (PendingCallback, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[eventID]
	if ok {
		delete(s.pending, eventID)
	}
	return p, ok
}
