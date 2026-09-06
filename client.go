//go:build wasm

package sse

import (
	"syscall/js"

	"webtyp.com/fmt"
)

// SSEClient is the SSE client for WASM.
type SSEClient struct {
	tinySSE           *tinySSE
	config            *ClientConfig
	handler           func(msg *SSEMessage)
	errorHandler      func(err error)
	es                js.Value
	reconnectAttempts int
	lastEventID       string
}

// Client creates a new SSEClient instance.
func (t *tinySSE) Client(c *ClientConfig) *SSEClient {
	return &SSEClient{
		tinySSE: t,
		config:  c,
	}
}

// Connect establishes a connection to the SSE endpoint.
func (c *SSEClient) Connect() {
	// 1. Construct URL with Last-Event-ID if available
	// The browser automatically sends Last-Event-ID header on reconnection,
	// but for our manual reconnections or initial connections we might want to append it?
	// Actually, EventSource API doesn't allow setting headers.
	// Browsers handle Last-Event-ID header automatically if the connection was dropped and browser retries.
	// But if WE close and reopen (e.g. for token refresh, which we removed), we might need to pass it in query?
	// The prompt removed auth/token logic.
	// Standard SSE: browser handles Last-Event-ID header for native retries.
	// If we manually retry, we might lose it unless we pass it.
	// But wait, the prompt says "tinysse handles connection, reconnection".
	// And "No Authentication". So we don't need to refresh tokens.
	// So maybe we can rely on browser's native reconnection for network issues?
	// However, `ClientConfig` has `RetryInterval` and `MaxRetryDelay`.
	// Browsers have their own retry mechanism.
	// The existing code had manual reconnection logic.
	// I will keep manual reconnection logic just in case, or to respect config.
	//
	// Note on Last-Event-ID: Browser sends it automatically in HTTP header `Last-Event-ID`.
	// We don't need to append it to URL usually.

	url := c.config.Endpoint
	c.es = js.Global().Get("EventSource").New(url)

	c.es.Set("onmessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.reconnectAttempts = 0 // Reset on successful message

		event := args[0]

		// Parse SSE fields
		// "data" is a string property of the event
		// "lastEventId" is a string property
		// "type" is the event type (e.g. "message", "update")

		dataStr := event.Get("data").String()
		eventID := event.Get("lastEventId").String()
		eventType := event.Get("type").String()

		// Update internal lastEventID
		if eventID != "" {
			c.lastEventID = eventID
		}

		if c.handler != nil {
			msg := &SSEMessage{
				Id:    eventID,
				Event: eventType,
				Data:  []byte(dataStr), // Raw bytes from string
			}
			c.handler(msg)
		}
		return nil
	}))

	c.es.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Event parsing
		// args[0] is the event.
		// readyState: 0=CONNECTING, 1=OPEN, 2=CLOSED
		readyState := c.es.Get("readyState").Int()

		if c.errorHandler != nil {
			// Construct a meaningful error
			// We can't get much detail from EventSource error event in browser for security reasons often.
			// But we can report state.
			c.errorHandler(fmt.Err("SSE connection error", "readyState", readyState))
		}

		// If CLOSED (2), browser gave up (e.g. fatal error). We can try manual reconnect.
		if readyState == 2 {
			c.reconnect()
		}
		return nil
	}))

	// Open handler to reset attempts?
	c.es.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.reconnectAttempts = 0
		return nil
	}))
}

// Close closes the SSE connection.
func (c *SSEClient) Close() {
	if !c.es.IsUndefined() && !c.es.IsNull() {
		c.es.Call("close")
	}
}

// OnMessage sets the handler for incoming messages.
func (c *SSEClient) OnMessage(handler func(msg *SSEMessage)) {
	c.handler = handler
}

// OnError sets the handler for errors.
func (c *SSEClient) OnError(handler func(err error)) {
	c.errorHandler = handler
}

func (c *SSEClient) reconnect() {
	c.Close()

	if c.config.MaxReconnectAttempts > 0 && c.reconnectAttempts >= c.config.MaxReconnectAttempts {
		if c.errorHandler != nil {
			c.errorHandler(fmt.Err("max reconnect attempts reached"))
		}
		return
	}

	delay := c.config.RetryInterval * (1 << c.reconnectAttempts)
	if delay > c.config.MaxRetryDelay {
		delay = c.config.MaxRetryDelay
	}
	if delay <= 0 {
		delay = 1000 // Default 1s if misconfigured
	}

	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.Connect()
		return nil
	}), delay)

	c.reconnectAttempts++
}
