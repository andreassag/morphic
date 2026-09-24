package events

import (
	"context"
	"sync"
	"time"
)

// Event represents a system-wide typed event broadcast to SSE clients.
type Event struct {
	Topic     string      `json:"topic"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

type subscriber struct {
	id     uint64
	topics map[string]bool
	ch     chan Event
}

// Bus is an in-memory pub/sub event bus with topic filtering.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber
	nextID      uint64
}

// DefaultBus is the shared application-wide event bus instance.
var DefaultBus = NewBus()

// NewBus creates a new Bus instance.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[uint64]*subscriber),
	}
}

// Subscribe returns a channel that receives events for the specified topics.
// When ctx is cancelled, the subscriber is automatically unregistered and channel closed.
func (b *Bus) Subscribe(ctx context.Context, topics ...string) <-chan Event {
	b.mu.Lock()
	b.nextID++
	id := b.nextID

	topicMap := make(map[string]bool)
	for _, t := range topics {
		topicMap[t] = true
	}

	ch := make(chan Event, 64)
	sub := &subscriber{
		id:     id,
		topics: topicMap,
		ch:     ch,
	}
	b.subscribers[id] = sub
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		if s, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(s.ch)
		}
		b.mu.Unlock()
	}()

	return ch
}

// Publish broadcasts an event to all subscribers listening on that topic (or wildcard "*").
func (b *Bus) Publish(topic, eventType string, payload interface{}) {
	evt := Event{
		Topic:     topic,
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if len(sub.topics) == 0 || sub.topics[topic] || sub.topics["*"] {
			select {
			case sub.ch <- evt:
			default:
				// Non-blocking: drop if subscriber buffer is full
			}
		}
	}
}
