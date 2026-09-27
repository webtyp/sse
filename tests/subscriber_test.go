//go:build wasm

package sse_test

import (
	"testing"

	"webtyp.com/events"
	. "webtyp.com/sse"
)

func TestSubscriber_DeliversTopicWithNilPayload(t *testing.T) {
	getInstances := setupMockEventSource()

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/events"})
	sub := Subscriber{Client: client}

	var receivedEvent *events.Event
	var callCount int

	sub.Subscribe("chat_room.inbox.u1", func(e events.Event) {
		callCount++
		receivedEvent = &e
	})

	client.Connect()

	instances := getInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	es := instances[0]
	event := makeFakeEvent("chat_room.inbox.u1", "evt-99", "")
	dispatchNamedEvent(es, "chat_room.inbox.u1", event)

	if callCount != 1 {
		t.Fatalf("expected handler to be called once, got %d", callCount)
	}

	if receivedEvent == nil {
		t.Fatal("expected event, got nil")
	}

	if receivedEvent.Topic != "chat_room.inbox.u1" {
		t.Errorf("expected topic 'chat_room.inbox.u1', got %q", receivedEvent.Topic)
	}

	if receivedEvent.Payload != nil {
		t.Errorf("expected nil Payload, got %v", receivedEvent.Payload)
	}
}
