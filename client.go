//go:build wasm

package sse

import (
	"syscall/js"

	"webtyp.com/fmt"
)

type eventListener struct {
	name     string
	handler  func(msg *SSEMessage)
	fn       js.Func
	attached bool
}

// SSEClient is the SSE client for WASM.
type SSEClient struct {
	tinySSE           *tinySSE
	config            *ClientConfig
	handler           func(msg *SSEMessage)
	errorHandler      func(err error)
	es                js.Value
	reconnectAttempts int
	lastEventID       string
	listeners         []*eventListener
}

// Client creates a new SSEClient instance.
func (t *tinySSE) Client(c *ClientConfig) *SSEClient {
	return &SSEClient{
		tinySSE: t,
		config:  c,
	}
}

func (c *SSEClient) parseSSEEvent(event js.Value) *SSEMessage {
	c.reconnectAttempts = 0 // Reset on successful message

	dataStr := event.Get("data").String()
	eventID := event.Get("lastEventId").String()
	eventType := event.Get("type").String()

	if eventID != "" {
		c.lastEventID = eventID
	}

	return &SSEMessage{
		Id:    eventID,
		Event: eventType,
		Data:  []byte(dataStr),
	}
}

// OnEvent registers a handler for SSE frames whose event name is name
// (frames published with PublishEvent or through sse.Publisher).
// OnMessage keeps receiving only unnamed frames. Handlers registered before
// or after Connect both work, and survive reconnects.
func (c *SSEClient) OnEvent(name string, handler func(msg *SSEMessage)) {
	el := &eventListener{
		name:    name,
		handler: handler,
	}
	c.listeners = append(c.listeners, el)

	if !c.es.IsUndefined() && !c.es.IsNull() {
		c.attachListener(el)
	}
}

func (c *SSEClient) attachListener(el *eventListener) {
	if el.attached {
		return
	}
	el.fn = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		msg := c.parseSSEEvent(args[0])
		if el.handler != nil {
			el.handler(msg)
		}
		return nil
	})
	c.es.Call("addEventListener", el.name, el.fn)
	el.attached = true
}

// Connect establishes a connection to the SSE endpoint.
func (c *SSEClient) Connect() {
	url := c.config.Endpoint
	c.es = js.Global().Get("EventSource").New(url)

	c.es.Set("onmessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		msg := c.parseSSEEvent(args[0])
		if c.handler != nil {
			c.handler(msg)
		}
		return nil
	}))

	c.es.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		readyState := c.es.Get("readyState").Int()

		if c.errorHandler != nil {
			c.errorHandler(fmt.Err("SSE connection error", "readyState", readyState))
		}

		if readyState == 2 {
			c.reconnect()
		}
		return nil
	}))

	c.es.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.reconnectAttempts = 0
		return nil
	}))

	// Re-attach all listeners to the new EventSource instance
	for _, el := range c.listeners {
		el.attached = false
		c.attachListener(el)
	}
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
