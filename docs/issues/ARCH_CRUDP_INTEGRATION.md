# TinySSE Integration with CRUDP

## Context
This document defines how `tinysse` integrates with `crudp` and `goserver` while maintaining strict separation of concerns.

## Design Principles
1. **tinysse = Pure SSE Transport**: Handles connections, reconnections, message delivery.
2. **crudp = User/Session Management**: Knows who is connected, routes messages.
3. **goserver = HTTP Routing**: Mounts tinysse as an HTTP handler.

## Decisions Made
- **ClientID**: One unique ID per SSE connection (tab). Multiple tabs = multiple clientIDs.
- **SSEServer**: Implements `http.Handler`. Consumer mounts it where needed.
- **Authentication**: Via cookies (automatic with SPA/PWA after login).
- **Channel Resolution**: Via `ChannelProvider` interface (similar to `UserProvider`).
- **Method names**: `Publish()` and `PublishEvent()` for broadcasting.
- **ClientID visibility**: Internal only, not exposed to client.
- **ID generation**: Internal correlative using tinystring `Convert(n).String()`.
- **Newlines**: Handled internally. tinysse splits data by `\n` and sends multiple `data:` lines.
- **HandlerID**: crudp responsibility. Include in Data or use Event field.
- **Errors**: Use tinystring `Err()` instead of standard `errors.New()`.

## SSEPublisher Interface

crudp uses this interface to publish messages. tinysse implements it.

```go
// crudp/interfaces.go (!wasm)

// SSEPublisher allows publishing messages to SSE clients.
// Implemented by tinysse.SSEServer.
type SSEPublisher interface {
    // Publish sends data to clients subscribed to the specified channels.
    // Data can contain newlines - tinysse handles them internally.
    Publish(data []byte, channels ...string)
    
    // PublishEvent sends data with an event type for client-side routing.
    PublishEvent(event string, data []byte, channels ...string)
}
```

### crudp Integration

```go
// crudp/crudp.go

type CrudP struct {
    // ... existing fields
    sse SSEPublisher // Injected dependency
}

// SetSSE injects the SSE publisher
func (cp *CrudP) SetSSE(sse SSEPublisher) {
    cp.sse = sse
}

// routeToSSE sends data to SSE clients (called from packet processing)
func (cp *CrudP) routeToSSE(data []byte, broadcast []string, handlerID uint8) {
    if cp.sse == nil {
        return
    }
    
    // Use event as handlerID for client-side routing
    event := Convert(handlerID).String()
    cp.sse.PublishEvent(event, data, broadcast...)
}
```

## ChannelProvider Interface

tinysse defines this interface in `interfaces_server.go` (with `//go:build !wasm`).
The consumer (crudp/session handler) implements it.

```go
//go:build !wasm

// tinysse/interfaces_server.go

package tinysse

import "net/http"

// ChannelProvider resolves SSE channels for a connection.
// Implemented by external packages (e.g., crudp session handler).
type ChannelProvider interface {
    // ResolveChannels extracts channels for an SSE connection.
    // Called once when client connects.
    //
    // Parameters:
    //   - r: The HTTP request (contains cookies, headers, query params)
    //
    // Returns:
    //   - channels: List of channels to subscribe (e.g., ["all", "user:123", "role:admin"])
    //   - err: If non-nil, connection is rejected with 401/403
    ResolveChannels(r *http.Request) (channels []string, err error)
}
```

### Default ChannelProvider

If `ServerConfig.ChannelProvider` is nil, tinysse uses a default that rejects all connections:

```go
import . "webtyp.com/fmt"

type defaultChannelProvider struct{}

func (d *defaultChannelProvider) ResolveChannels(r *http.Request) ([]string, error) {
    return nil, Err("channel provider not configured")
}
```

## Data Format Rules

### Newline Handling (Hybrid Approach)

**tinysse handles newlines internally** by splitting data and sending multiple `data:` lines:

```go
// tinysse/hub.go (!wasm)
import (
    "bytes"
    "strings"
    . "webtyp.com/fmt"
)

func formatSSEMessage(id, event string, data []byte) string {
    var b strings.Builder
    b.WriteString("id: ")
    b.WriteString(id)
    b.WriteString("\n")
    
    if event != "" {
        b.WriteString("event: ")
        b.WriteString(event)
        b.WriteString("\n")
    }
    
    // Split data by \n (also handles \r\n)
    // Each line gets "data: " prefix
    lines := bytes.Split(data, []byte("\n"))
    for _, line := range lines {
        // Remove trailing \r if present
        line = bytes.TrimSuffix(line, []byte("\r"))
        b.WriteString("data: ")
        b.Write(line)
        b.WriteString("\n")
    }
    
    b.WriteString("\n") // End of message
    return b.String()
}
```

**Example**:
```
Input:  {"text": "Hello\nWorld"}

Output (SSE stream):
id: 1
data: {"text": "Hello
data: World"}

Browser receives: {"text": "Hello\nWorld"}  ← Identical to input
```

**Why this approach?**
- ✅ Standard SSE protocol
- ✅ No validation needed - any data works
- ✅ No code added to WASM client (browser handles reconstruction)
- ✅ Caller doesn't need to know about SSE format

### ID Generation

tinysse generates SSE IDs internally using correlative numbers:

```go
// Using tinystring instead of strconv
import . "webtyp.com/fmt"

func (h *hub) nextID() string {
    h.lastID++
    return Convert(h.lastID).String()
}
```

## Full Integration Example

### Server Setup

```go
// main.go (server)

func main() {
    // 1. Create session handler (implements ChannelProvider)
    sessionHandler := &session.SessionHandler{
        sessions: mySessionStore,
    }
    
    // 2. Create SSE server
    sseServer := tinysse.New(&tinysse.Config{
        Log: log.Println,
    }).Server(&tinysse.ServerConfig{
        ClientChannelBuffer: 50,
        HistoryReplayBuffer: 100,
        ChannelProvider:     sessionHandler,
    })
    
    // 3. Create crudp and inject SSE
    cp := crudp.New()
    cp.SetSSE(sseServer) // Inject SSEPublisher
    cp.RegisterHandler(modules.Init()...)
    
    // 4. Mount handlers
    mux := http.NewServeMux()
    mux.Handle("/events", sseServer)
    mux.Handle("/api", cp.Router())
    
    http.ListenAndServe(":8080", mux)
}
```

### Client Setup (WASM)

```go
// web/client.go (wasm)

func main() {
    sseClient := tinysse.New(&tinysse.Config{
        Log: console.Log,
    }).Client(&tinysse.ClientConfig{
        Endpoint:      "/events",
        RetryInterval: 1000,
    })
    
    sseClient.OnMessage(func(msg *tinysse.SSEMessage) {
        // msg.Event contains handlerID (if crudp used PublishEvent)
        // msg.Data contains the payload (newlines reconstructed by browser)
        switch msg.Event {
        case "1": // handlerID 1
            handleUserEvent(msg.Data)
        case "2": // handlerID 2
            handleOrderEvent(msg.Data)
        default:
            handleGenericMessage(msg.Data)
        }
    })
    
    sseClient.OnError(func(err error) {
        console.Error("SSE:", err)
    })
    
    sseClient.Connect()
    
    // Later, to disconnect:
    // sseClient.Close()
    
    select {}
}
```

## SSEClient Methods

```go
// client.go (wasm)

type SSEClient struct {
    *tinySSE
    config       *ClientConfig
    handler      func(msg *SSEMessage)
    errorHandler func(err error)
    es           js.Value // EventSource
    // ...
}

// Connect establishes connection to SSE endpoint
func (c *SSEClient) Connect()

// Close disconnects the EventSource
func (c *SSEClient) Close() {
    if !c.es.IsUndefined() && !c.es.IsNull() {
        c.es.Call("close")
    }
}

// OnMessage sets the message handler
func (c *SSEClient) OnMessage(handler func(msg *SSEMessage))

// OnError sets the error handler
func (c *SSEClient) OnError(handler func(err error))
```

## Authentication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│  1. User logs in via crudp                                      │
│     └─▶ POST /api (login action)                                │
│     └─▶ Server sets HttpOnly cookie                             │
│                                                                 │
│  2. App connects to SSE                                         │
│     └─▶ new EventSource("/events")                              │
│     └─▶ Browser sends cookie automatically                     │
│                                                                 │
│  3. tinysse receives request                                    │
│     └─▶ ChannelProvider.ResolveChannels(r)                     │
│     └─▶ Session handler validates cookie                       │
│     └─▶ Returns channels ["all", "user:john"]                  │
│                                                                 │
│  4. Connection established with channels                        │
│                                                                 │
│  5. Business logic triggers broadcast                           │
│     └─▶ crudp.routeToSSE(data, ["user:john"], handlerID)       │
│     └─▶ sseServer.PublishEvent("1", data, "user:john")         │
│                                                                 │
│  6. Client receives message                                     │
│     └─▶ OnMessage: msg.Event="1", msg.Data=payload             │
│                                                                 │
│  7. User logs out or closes tab                                 │
│     └─▶ sseClient.Close() or browser closes connection         │
└─────────────────────────────────────────────────────────────────┘
```
