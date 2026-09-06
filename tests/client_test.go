//go:build wasm

package sse_test

import (
	. "webtyp.com/sse"
	"syscall/js"
	"testing"
)

// This test requires `wasmbrowsertest` or a similar environment.
// If running in standard `go test`, it will be skipped by build tag.

func TestClientConnect(t *testing.T) {
	// We cannot easily spin up a real server in WASM environment.
	// Tests here typically verify JS interop or logic that doesn't require network
	// OR use a mock EventSource if we can inject it.
	// Since we use global `EventSource`, we can mock it in JS global scope!

	// Mock EventSource
	var esCreated bool
	js.Global().Set("EventSource", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Verify URL argument
		if len(args) > 0 && args[0].String() == "/events" {
			esCreated = true
		}

		// Return a valid object so Connect doesn't falter
		obj := js.Global().Get("Object").New()
		obj.Set("readyState", 0)
		obj.Set("close", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return nil }))
		return obj
	}))

	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)
	client := tSSE.Client(&ClientConfig{
		Endpoint: "/events",
	})

	client.Connect()

	// Verify Connect called New
	if !esCreated {
		t.Fatal("EventSource constructor not called with expected URL")
	}
}

func TestClientOnMessage(t *testing.T) {
	// Setup mock to capture the EventSource instance
	var esInstance js.Value

	// Mock EventSource to capture onmessage
	js.Global().Set("EventSource", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Create a real JS object so we can keep a reference to it
		obj := js.Global().Get("Object").New()
		obj.Set("readyState", 0)
		obj.Set("close", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return nil }))

		esInstance = obj
		return obj
	}))

	tSSE := New(&Config{})
	client := tSSE.Client(&ClientConfig{Endpoint: "/test"})

	var received *SSEMessage
	client.OnMessage(func(msg *SSEMessage) {
		received = msg
	})

	client.Connect()

	if esInstance.IsUndefined() {
		t.Fatal("EventSource instance was not created")
	}

	onMessage := esInstance.Get("onmessage")
	if onMessage.IsUndefined() {
		t.Fatal("onmessage handler not set")
	}

	// Simulate incoming message
	// Event object mock
	// We need a JS object with 'data', 'lastEventId', 'type' properties
	event := js.Global().Get("Object").New()
	event.Set("data", "hello world")
	event.Set("lastEventId", "123")
	event.Set("type", "test-event")

	onMessage.Invoke(event)

	if received == nil {
		t.Fatal("handler not called")
	}

	verifyMessage(t, received, "test-event", []byte("hello world"))
	if received.Id != "123" {
		t.Errorf("expected ID '123', got %s", received.Id)
	}
}
