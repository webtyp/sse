# Configuration

`tinysse` uses a split configuration pattern to separate shared settings from environment-specific settings (Server vs Client/WASM).

## Shared Configuration

The `Config` struct is used by both Server and Client.

- **Config Struct Definition**: [tinysse/config.go](../config.go)

## Server Configuration

The `ServerConfig` struct is used when initializing the server with `.Server()`. It is only available in `!wasm` builds.

- **ServerConfig Struct Definition**: [tinysse/server_config.go](../server_config.go)
- **ChannelProvider Interface**: [tinysse/interfaces_server.go](../interfaces_server.go)

### Key Options

- **Path**: The stream URL path, e.g. `"/events"`. Required by `MountAPI`.
- **Access**: Access level for opening the stream (`model.AccessPublic`, `model.AccessAuthenticated`, or `model.AccessGuarded`).
- **Resource**: The permission checked when `Access` is `model.AccessGuarded`. Must be empty unless `Access` is `model.AccessGuarded`.
- **ClientChannelBuffer**: Controls the size of the Go channel for each connected client. Increase this if you send bursts of messages to prevent blocking.
- **HistoryReplayBuffer**: Determines how many recent messages are stored for replay when a client reconnects with `Last-Event-ID`.
- **ChannelProvider**: A required interface implementation that resolves which channels a client should be subscribed to based on the HTTP request. Required by `MountAPI`.

## Client Configuration

The `ClientConfig` struct is used when initializing the client with `.Client()`. It is only available in `wasm` builds.

- **ClientConfig Struct Definition**: [tinysse/client_config.go](../client_config.go)

### Key Options

- **Endpoint**: The URL of the SSE server (e.g., `/events`).
- **RetryInterval**: First wait in milliseconds before reconnecting; must be greater than zero.
- **MaxRetryDelay**: Longest wait in milliseconds; must be at least `RetryInterval`. The wait doubles after each failed attempt from `RetryInterval` up to this value (the policy is `webtyp.com/backoff`). An invalid pair is reported through `OnError` and the client does not reconnect.
- **MaxReconnectAttempts**: Limit on how many times to retry before giving up (0 = unlimited).

