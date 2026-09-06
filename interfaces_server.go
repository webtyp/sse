package sse

import "webtyp.com/router"

// ChannelProvider resolves SSE channels for a connection.
// Implemented by external packages (e.g., crudp session handler).
type ChannelProvider interface {
	// ResolveChannels extracts channels for an SSE connection.
	// Called once when client connects.
	//
	// Parameters:
	//   - ctx: The router context (contains headers, path, cookies)
	//
	// Returns:
	//   - channels: List of channels to subscribe (e.g., ["all", "user:123", "role:admin"])
	//   - err: If non-nil, connection is rejected with 401/403
	ResolveChannels(ctx router.Context) (channels []string, err error)
}

// SSEPublisher allows publishing messages to SSE clients.
// Implemented by sse.SSEServer.
type SSEPublisher interface {
	// Publish sends data to clients subscribed to the specified channels.
	// Data can contain newlines - tinysse handles them internally.
	Publish(data []byte, channels ...string)

	// PublishEvent sends data with an event type for client-side routing.
	PublishEvent(event string, data []byte, channels ...string)
}
