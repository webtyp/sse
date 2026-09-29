# Usage Guide

This guide covers how to install and use `webtyp.com/sse` for both server-side (Go) and client-side (TinyGo/WASM).

## Installation

```bash
go get webtyp.com/sse
```

## Server-Side Implementation

The server component handles HTTP connections, channel resolution, and broadcasting. `SSEServer` implements `router.APIModule`. Each SSE stream terminates automatically when the client disconnects.

### 1. Setup & Mounting

Create a new `SSEServer` using `New()` and `Server()`. You must provide a `ServerConfig` including `Path`, `Access`, and `ChannelProvider`.

```go
package main

import (
	"log"

	"webtyp.com/model"
	"webtyp.com/sse"
)

func main() {
	// 1. Shared Config (Optional Logger)
	cfg := &sse.Config{
		Log: log.Println,
	}

	// 2. Server Config
	serverCfg := &sse.ServerConfig{
		Path:                "/events",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 100,
		HistoryReplayBuffer: 50,
		ChannelProvider:     &MyChannelProvider{}, // See below
	}

	// 3. Initialize Server
	sseServer := sse.New(cfg).Server(serverCfg)

	// 4. Mount API route automatically with the router
	sseServer.MountAPI(r)
}
```

`SSEServer` mounts itself as a `router.APIModule` via `MountAPI(r)`.

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

Use the `Publish` or `PublishEvent` methods to send messages to subscribed clients, or adapt with `sse.Publisher` (`events.Publisher`).

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
	cfg := &sse.Config{
		Log: func(args ...any) { fmt.Println(args...) },
	}

	// 2. Client Config
	clientCfg := &sse.ClientConfig{
		Endpoint:             "/events",
		RetryInterval:        1000, // 1 second
		MaxRetryDelay:        5000,
		MaxReconnectAttempts: 10,
	}

	// 3. Initialize Client
	client := sse.New(cfg).Client(clientCfg)

	// 4. Set Handlers
	client.OnMessage(func(msg *sse.SSEMessage) {
		fmt.Printf("Received ID: %s, Event: %s\n", msg.Id, msg.Event)
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

### 2. Receiving Named Events

Browsers deliver named SSE events (frames published with `PublishEvent` or `sse.Publisher`) exclusively to named event listeners. `OnMessage` receives unnamed frames (`type: "message"`).

Register callbacks for named events with `OnEvent`:

```go
client.OnEvent("chat_room.inbox.u1", func(msg *sse.SSEMessage) {
	fmt.Printf("Notification for chat inbox: %s\n", string(msg.Data))
})
```

Handlers registered with `OnEvent` work before or after `Connect()` and survive reconnects.

### 3. Events Subscriber Adapter

You can adapt an `*SSEClient` to `events.Subscriber` using `sse.Subscriber{Client: client}`:

```go
sub := sse.Subscriber{Client: client}
sub.Subscribe("chat_room.inbox.u1", func(e events.Event) {
	// e.Topic is "chat_room.inbox.u1"
	// e.Payload is nil
	// Refetch typed data via router.Caller
})
```

**Nil-Payload Contract**: `Subscriber` delivers the TOPIC only (`e.Payload` is always `nil`). A subscriber cannot know which concrete model a topic carries; the intended pattern is notification ("topic X changed") followed by fetching typed data.

### 4. Reconnection

The library handles reconnection automatically based on `RetryInterval`. It also respects `Last-Event-ID` to resume the stream, ensuring no data loss during brief disconnects. Registered `OnEvent` listeners automatically re-attach to the new connection.
