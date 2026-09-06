# Usage Guide

This guide covers how to install and use `tinysse` for both server-side (Go) and client-side (TinyGo/WASM).

## Installation

```bash
go get webtyp.com/sse
```

## Server-Side Implementation

The server component handles HTTP connections, channel resolution, and broadcasting.

### 1. Setup

Create a new `SSEServer` using `New()` and `Server()`. You must provide a `ServerConfig`.

```go
package main

import (
	"log"

	"webtyp.com/router"
	"webtyp.com/sse"
)

func main() {
	// 1. Shared Config (Optional Logger)
	cfg := &tinysse.Config{
		Log: log.Println,
	}

	// 2. Server Config
	serverCfg := &tinysse.ServerConfig{
		ClientChannelBuffer: 100,
		HistoryReplayBuffer: 50,
		ChannelProvider:     &MyChannelProvider{}, // See below
	}

	// 3. Initialize Server
	sseServer := tinysse.New(cfg).Server(serverCfg)

	// 4. Mount as a streaming route. Stream() is what guarantees the handler
	//    gets a Streamer (a Context that can Flush) — no capability assertion.
	r.Stream("/events", sseServer.StreamHandler())
}
```

`StreamHandler()` returns a `router.StreamFunc`, so the transport is supplied by whatever `router` implementation you mount (backend or WASM). The library never names `net/http`.

### 2. Channel Resolution

You must implement the `ChannelProvider` interface to determine which channels a connecting client subscribes to. This is typically based on authentication (cookies, headers).

```go
type MyChannelProvider struct{}

func (p *MyChannelProvider) ResolveChannels(ctx router.Context) ([]string, error) {
	// Example: Extract user ID from cookie or session, via ctx.GetHeader/ctx.Path
	userID := "user_123" // Replace with real auth logic
	role := "admin"

	return []string{"all", "user:" + userID, "role:" + role}, nil
}
```

### 3. Broadcasting Messages

Use the `Publish` or `PublishEvent` methods to send messages to subscribed clients.

```go
// Send a simple message to channel "all"
sseServer.Publish([]byte("Hello everyone!"), "all")

// Send a named event to specific user
data := []byte(`{"status": "updated"}`)
sseServer.PublishEvent("update", data, "user:user_123")
```

- **Publish**: Sends a message without an event name (defaults to "message" in browser).
- **PublishEvent**: Sends a message with a specific `event:` field.

---

## Client-Side Implementation (WASM)

The client component connects to the SSE server and handles incoming messages. It is designed for TinyGo.

### 1. Setup

Create a new `SSEClient` using `New()` and `Client()`.

```go
package main

import (
	"fmt"
	"webtyp.com/sse"
)

func main() {
	// 1. Shared Config
	cfg := &tinysse.Config{
		Log: func(args ...any) { fmt.Println(args...) },
	}

	// 2. Client Config
	clientCfg := &tinysse.ClientConfig{
		Endpoint:             "/events",
		RetryInterval:        1000, // 1 second
		MaxRetryDelay:        5000,
		MaxReconnectAttempts: 10,
	}

	// 3. Initialize Client
	client := tinysse.New(cfg).Client(clientCfg)

	// 4. Set Handlers
	client.OnMessage(func(msg *tinysse.SSEMessage) {
		fmt.Printf("Received ID: %s, Event: %s\n", msg.ID, msg.Event)
		fmt.Printf("Data: %s\n", string(msg.Data))
	})

	client.OnError(func(err error) {
		fmt.Printf("SSE Error: %v\n", err)
	})

	// 5. Connect
	client.Connect()

	// Keep the main function running
	select {}
}
```

### 2. Handling Messages

The `OnMessage` callback receives an `*SSEMessage` struct.

- **Data**: The payload is raw `[]byte`. You are responsible for parsing it (e.g., JSON unmarshal).
- **Event**: The event name (e.g., "update", "alert").
- **ID**: The message ID.

### 3. Reconnection

The library handles reconnection automatically based on `RetryInterval`. It also respects the `Last-Event-ID` to resume the stream from the last received message, ensuring no data loss during brief disconnects.
