# TinySSE Architecture

> **Package:** `webtyp.com/sse`

## System Overview

```mermaid
flowchart TB
    subgraph Browser["🌐 Browser (WASM)"]
        APP["Go App\n(TinyGo WASM)"]
        ES["EventSource\nWrapper"]
        TP["TokenProvider"]
    end
    
    subgraph Server["🖥️ Server (Go)"]
        HUB["SSEHub"]
        TV["TokenValidator"]
        BUF["Message Buffer"]
        CLIENTS["Clients Map"]
    end
    
    subgraph CRUDP["📦 CRUDP Integration"]
        HANDLER["Handler\nCreate/Update/Delete"]
        RESPONSE["Response()\nbroadcast: []string"]
    end
    
    APP -->|"1. Request Logic"| TP
    TP -->|"2. POST /auth/sse-token"| TV
    TV -->|"3. SSE Token (Short-lived)"| TP
    
    APP -- "4. Manual Connect" --> ES
    ES -->|"5. GET /events?token=x&lastEventId=y"| HUB
    
    HUB -->|"6. Validate"| TV
    HUB -->|"7. Register"| CLIENTS
    
    HANDLER -->|"8. Response(data, broadcast)"| RESPONSE
    RESPONSE -->|"9. routeToSSE()"| HUB
    HUB -->|"10. Store in"| BUF
    HUB -->|"11. Broadcast"| CLIENTS
    CLIENTS -->|"12. SSE data: {payload}"| ES
    ES -->|"13. OnMessage (JSON Parse)"| APP
```

## Connection Flow (Hybrid Reconnection)

```mermaid
sequenceDiagram
    participant App as Go App (WASM)
    participant ES as JS Wrapper
    participant Server as SSE Server
    participant Hub as SSEHub

    Note over App,Hub: 1. Initial Connection
    App->>Server: POST /auth/sse-token
    Server-->>App: Token T1
    App->>ES: New EventSource(url + "?token=T1")
    ES->>Server: Connection Request
    Server-->>ES: 200 OK (Stream Open)

    Note over App,Hub: 2. Network Drop (Native Retry)
    ES->>ES: Network Error
    ES-->>ES: Wait retryInterval
    ES->>Server: Reconnect (same URL/Token)
    Server-->>ES: 200 OK (Resumed)

    Note over App,Hub: 3. Token Expiry (Manual Rotation)
    ES->>ES: Net Error -> Retry
    ES->>Server: Reconnect (Token T1 Expired)
    Server-->>ES: 401 Unauthorized
    ES->>App: OnError(AuthError/Close)
    App->>Server: POST /auth/sse-token
    Server-->>App: Token T2
    App->>ES: New EventSource(url + "?token=T2&lastEventId=N")
    ES->>Server: Connection Request
    Server->>Hub: GetMessagesSince(N)
    Hub-->>Server: Missed Messages
    Server-->>ES: Replay + Stream Open
```

## Hub Architecture (Server-Only)

```mermaid
flowchart LR
    subgraph SSEHub ["SSEHub (!wasm)"]
        MU["sync.RWMutex"]
        CLIENTS["map[string]*SSEClient"]
        BUFFER["[]SSEMessage"]
    end
    
    subgraph SSEClient
        ID["ID string"]
        UID["UserID string"]
        ROLE["Role string"]
        CHAN["Channels []string"]
        SEND["Send chan []byte"]
    end
    
    subgraph SharedModels ["models.go"]
        DTO["SSEMessage DTO"]
    end
    
    CLIENTS --> SSEClient
    SSEHub --> SharedModels
```

## File Structure & Build Constraints

```mermaid
flowchart TB
    subgraph Shared["Shared Code (wasm & !wasm)"]
        TINYSSE["tinysse.go\nNew(), Config"]
        MODELS["models.go\nSSEMessage, SSEError"]
    end
    
    subgraph ServerOnly["//go:build !wasm"]
        SERVER["server.go\nrouter.StreamFunc"]
        HUB["hub.go\nSSEHub Logic"]
        CLIENT["client.go\nSSEClient Struct"]
    end
    
    subgraph WASMOnly["//go:build wasm"]
        WASM["wasm.go\nEventSource Wrapper\nReconnect Logic"]
    end
    
    TINYSSE --> MODELS
    
    SERVER --> HUB
    SERVER --> MODELS
    HUB --> CLIENT
    HUB --> MODELS
    
    WASM --> MODELS
```

## Key Design Decisions

| Aspect | Decision | Reason |
|--------|----------|--------|
| **Hub Location** | **Server-Only** | Reduce WASM binary size. Client is single-connection. |
| **Data Structure** | Map + Mutex | Fast lookup by ID in server. |
| **Reconnection** | **Hybrid** | Native for network glitches, Manual for token rotation. |
| **Protocol** | SSE Standard | `data: ...\n\n` required for browser compatibility. |
| **Auth** | Query Token | Compatible with EventSource API. |
| **Errors** | MessageType | Reuse properties of `tinystring.MessageType`. |
