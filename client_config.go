//go:build wasm

package sse

// ClientConfig holds configuration strictly for the Browser/WASM Client.
// An invalid pair is reported through OnError and the client does not reconnect.
type ClientConfig struct {
	// Endpoint is the SSE server URL.
	Endpoint string

	// RetryInterval: first wait in milliseconds before reconnecting; must be greater than zero
	RetryInterval int

	// MaxRetryDelay: longest wait in milliseconds; must be at least RetryInterval
	MaxRetryDelay int

	// MaxReconnectAttempts limits retry attempts. 0 = unlimited.
	MaxReconnectAttempts int
}
