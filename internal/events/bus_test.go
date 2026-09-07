package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/exterex/morphic/internal/events"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := events.NewBus()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subCh := bus.Subscribe(ctx, "converter")

	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish("converter", "progress", map[string]interface{}{"progress": 0.5})
	}()

	select {
	case evt := <-subCh:
		if evt.Topic != "converter" {
			t.Errorf("expected topic 'converter', got %q", evt.Topic)
		}
		if evt.Type != "progress" {
			t.Errorf("expected type 'progress', got %q", evt.Type)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBus_WildcardSubscription(t *testing.T) {
	bus := events.NewBus()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subCh := bus.Subscribe(ctx, "*")

	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish("dupfinder", "job_done", map[string]interface{}{"status": "done"})
	}()

	select {
	case evt := <-subCh:
		if evt.Topic != "dupfinder" {
			t.Errorf("expected topic 'dupfinder', got %q", evt.Topic)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for wildcard event")
	}
}
