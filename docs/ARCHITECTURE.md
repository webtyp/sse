# TinySSE Architecture

> **Package:** `webtyp.com/sse`

## System Overview

`SSEServer` implements `router.APIModule` (`ModelName()` and `MountAPI(r)`), mounting itself as a streaming route with configured path and access gate (`model.Access`).

```mermaid
flowchart TB
    subgraph Browser["🌐 Browser (WASM)"]
        APP["Go App\n(TinyGo WASM)"]
        ES["EventSource\nWrapper"]
        SUB["sse.Subscriber"]
    end
    
    subgraph Server["🖥️ Server (Go)"]
        PUB["sse.Publisher"]
        HUB["SSEHub"]
        BUF["Message Buffer"]
        CLIENTS["Clients Map"]
    end
    
    APP -->|"1. OnEvent / Subscribe"| ES
    ES -->|"2. GET /events"| HUB
    
    HUB -->|"3. Register"| CLIENTS
    
    PUB -->|"4. Publish(Event)"| HUB
    HUB -->|"5. Store in"| BUF
    HUB -->|"6. Broadcast"| CLIENTS
    CLIENTS -->|"7. SSE event: topic data: {payload}"| ES
    ES -->|"8. OnEvent Callback"| SUB
```

## Connection Flow (Hybrid Reconnection)

```mermaid
sequenceDiagram
    participant App as Go App (WASM)
    participant ES as JS Wrapper
    participant Server as SSE Server
    participant Hub as SSEHub

    Note over App,Hub: 1. Initial Connection
    App->>ES: New EventSource(url)
    ES->>Server: Connection Request (MountAPI route)
    Server-->>ES: 200 OK (Stream Open)

    Note over App,Hub: 2. Network Drop (Native Retry)
    ES->>ES: Network Error
    ES-->>ES: Wait retryInterval
    ES->>Server: Reconnect
    Server-->>ES: 200 OK (Resumed)
```

## File Structure & Build Constraints

```mermaid
flowchart TB
    subgraph Shared["Shared Code (wasm & !wasm)"]
        TINYSSE["tinysse.go\nNew(), Config"]
        MODELS["models.go\nSSEMessage"]
        CONFIG["server_config.go\nServerConfig, ModuleName"]
    end
    
    subgraph ServerOnly["//go:build !wasm"]
        SERVER["server.go\nSSEServer, MountAPI"]
        HUB["hub.go\nSSEHub Logic"]
        PUB["events_publisher.go\nPublisher Adapter"]
    end
    
    subgraph WASMOnly["//go:build wasm"]
        CLIENT["client.go\nSSEClient, OnEvent"]
        SUB["subscriber.go\nSubscriber Adapter"]
    end
    
    TINYSSE --> MODELS
    
    SERVER --> HUB
    SERVER --> MODELS
    HUB --> MODELS
    PUB --> SERVER
    
    CLIENT --> MODELS
    SUB --> CLIENT
```

## Key Design Decisions

| Aspect | Decision | Reason |
|--------|----------|--------|
| **Mounting** | **`router.APIModule`** | `MountAPI` registers streaming route with `Path`, `Access`, `Resource`. |
| **Browser Events** | **Named `addEventListener`** | Browser `OnMessage` only handles unnamed events; `OnEvent` registers named SSE handlers. |
| **Pub/Sub Adapters** | **`Publisher` & `Subscriber`** | Adapter pattern to `events.Publisher` and `events.Subscriber`. |
| **Hub Location** | **Server-Only** | Reduce WASM binary size. Client is single-connection. |
| **Protocol** | **SSE Standard** | `event: ...\ndata: ...\n\n` for standard browser `EventSource` dispatch. |
| **HTTP/2 Transport** | **No hop-by-hop headers** | RFC 9113 §8.2.2 prohibits `Connection` in HTTP/2. TinySSE emits only `Content-Type: text/event-stream` and `Cache-Control: no-cache`. |

