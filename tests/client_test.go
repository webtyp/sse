//go:build wasm

package sse_test

import (
	"syscall/js"
	"testing"

	. "webtyp.com/sse"
)

func setupMockEventSource() (getInstances func() []js.Value) {
	var instances []js.Value

	js.Global().Set("EventSource", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("readyState", 0)
		obj.Set("close", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			obj.Set("readyState", 2)
			return nil
		}))

		listeners := js.Global().Get("Array").New()
		obj.Set("_listeners", listeners)

		obj.Set("addEventListener", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			name := args[0].String()
			fn := args[1]
			pair := js.Global().Get("Object").New()
			pair.Set("name", name)
			pair.Set("fn", fn)
			listeners.Call("push", pair)
			return nil
		}))

		instances = append(instances, obj)
		return obj
	}))

	return func() []js.Value {
		return instances
	}
}

func dispatchNamedEvent(es js.Value, name string, eventObj js.Value) {
	listeners := es.Get("_listeners")
	if listeners.IsUndefined() || listeners.IsNull() {
		return
	}
	length := listeners.Get("length").Int()
	for i := 0; i < length; i++ {
		pair := listeners.Index(i)
		if pair.Get("name").String() == name {
			pair.Get("fn").Invoke(eventObj)
		}
	}
}

func makeFakeEvent(eventType, eventID, data string) js.Value {
	event := js.Global().Get("Object").New()
	event.Set("type", eventType)
	event.Set("lastEventId", eventID)
	event.Set("data", data)
	return event
}

func TestClientConnect(t *testing.T) {
	getInstances := setupMockEventSource()

	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)
	client := tSSE.Client(&ClientConfig{
		Endpoint: "/events",
	})

	client.Connect()

	instances := getInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 EventSource instance, got %d", len(instances))
	}
}

func TestClientOnMessage(t *testing.T) {
	getInstances := setupMockEventSource()

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/test"})

	var received *SSEMessage
	client.OnMessage(func(msg *SSEMessage) {
		received = msg
	})

	client.Connect()

	instances := getInstances()
	if len(instances) == 0 {
		t.Fatal("EventSource instance was not created")
	}

	es := instances[0]
	onMessage := es.Get("onmessage")
	if onMessage.IsUndefined() {
		t.Fatal("onmessage handler not set")
	}

	event := makeFakeEvent("message", "123", "hello world")
	onMessage.Invoke(event)

	if received == nil {
		t.Fatal("handler not called")
	}

	verifyMessage(t, received, "message", []byte("hello world"))
	if received.Id != "123" {
		t.Errorf("expected ID '123', got %s", received.Id)
	}
}

func TestClientOnEvent_BeforeConnect(t *testing.T) {
	getInstances := setupMockEventSource()

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/events"})

	var received *SSEMessage
	client.OnEvent("chat_room.inbox.u1", func(msg *SSEMessage) {
		received = msg
	})

	client.Connect()

	instances := getInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	es := instances[0]
	event := makeFakeEvent("chat_room.inbox.u1", "evt-1", "news available")
	dispatchNamedEvent(es, "chat_room.inbox.u1", event)

	if received == nil {
		t.Fatal("OnEvent handler was not called")
	}
	verifyMessage(t, received, "chat_room.inbox.u1", []byte("news available"))
	if received.Id != "evt-1" {
		t.Errorf("expected ID 'evt-1', got %s", received.Id)
	}
}

func TestClientOnEvent_AfterConnect(t *testing.T) {
	getInstances := setupMockEventSource()

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/events"})

	client.Connect()

	instances := getInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	var received *SSEMessage
	client.OnEvent("dynamic_topic", func(msg *SSEMessage) {
		received = msg
	})

	es := instances[0]
	event := makeFakeEvent("dynamic_topic", "evt-2", "dynamic payload")
	dispatchNamedEvent(es, "dynamic_topic", event)

	if received == nil {
		t.Fatal("OnEvent handler registered after Connect was not called")
	}
	verifyMessage(t, received, "dynamic_topic", []byte("dynamic payload"))
}

func TestClientOnEvent_SurvivesReconnect(t *testing.T) {
	getInstances := setupMockEventSource()

	// Mock setTimeout to immediately invoke callback
	js.Global().Set("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		cb := args[0]
		cb.Invoke()
		return nil
	}))

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{
		Endpoint:             "/events",
		RetryInterval:        10,
		MaxRetryDelay:        50,
		MaxReconnectAttempts: 3,
	})

	var received *SSEMessage
	client.OnEvent("reconnect_topic", func(msg *SSEMessage) {
		received = msg
	})

	client.Connect()

	instances := getInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance initially, got %d", len(instances))
	}

	firstES := instances[0]
	// Simulate connection loss with readyState = 2 (CLOSED)
	firstES.Set("readyState", 2)
	onError := firstES.Get("onerror")
	if !onError.IsUndefined() && !onError.IsNull() {
		onError.Invoke()
	}

	// Verify reconnect created a second EventSource instance
	instances = getInstances()
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances after reconnect, got %d", len(instances))
	}

	secondES := instances[1]
	event := makeFakeEvent("reconnect_topic", "evt-3", "after reconnect")
	dispatchNamedEvent(secondES, "reconnect_topic", event)

	if received == nil {
		t.Fatal("OnEvent handler was not called on reconnected EventSource")
	}
	verifyMessage(t, received, "reconnect_topic", []byte("after reconnect"))
}

func TestClientOnEvent_EventFiltering(t *testing.T) {
	getInstances := setupMockEventSource()

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/events"})

	var onMessageReceived *SSEMessage
	var onEventReceived *SSEMessage

	client.OnMessage(func(msg *SSEMessage) {
		onMessageReceived = msg
	})
	client.OnEvent("named_event", func(msg *SSEMessage) {
		onEventReceived = msg
	})

	client.Connect()

	instances := getInstances()
	es := instances[0]

	// 1. Dispatch named event
	namedEvt := makeFakeEvent("named_event", "101", "named payload")
	dispatchNamedEvent(es, "named_event", namedEvt)

	if onEventReceived == nil {
		t.Error("OnEvent handler should have been called for named event")
	}
	if onMessageReceived != nil {
		t.Error("OnMessage handler should NOT have been called for named event")
	}

	// Reset received holders
	onEventReceived = nil
	onMessageReceived = nil

	// 2. Dispatch unnamed (onmessage) event
	onMessage := es.Get("onmessage")
	unnamedEvt := makeFakeEvent("message", "102", "unnamed payload")
	onMessage.Invoke(unnamedEvt)

	if onMessageReceived == nil {
		t.Error("OnMessage handler should have been called for unnamed event")
	}
	if onEventReceived != nil {
		t.Error("OnEvent handler should NOT have been called for unnamed event")
	}
}
