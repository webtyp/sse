package sse

import "webtyp.com/model"

const ModuleName = "sse"

// ServerConfig holds configuration strictly for the SSE stream handler.
type ServerConfig struct {
	// ClientChannelBuffer prevents blocking on slow clients.
	// Recommended: 10-100.
	ClientChannelBuffer int

	// HistoryReplayBuffer manages the "Last-Event-ID" replay history.
	// Recommended: Depends on message frequency.
	HistoryReplayBuffer int

	// ReplayAllOnConnect replays the full history buffer to every new client
	// on first connect (when no Last-Event-ID is provided).
	// Useful for log viewers where clients may connect after events are published.
	ReplayAllOnConnect bool

	// ChannelProvider resolves channels for each SSE connection. Required by MountAPI.
	ChannelProvider ChannelProvider

	// Path is the stream URL, e.g. "/events". Required.
	Path string

	// Access is who may open the stream. The zero value, model.AccessGuarded,
	// requires an identity AND a permission on Resource (read).
	// model.AccessAuthenticated requires only an identity; model.AccessPublic, none.
	Access model.Access

	// Resource is the permission checked when Access is model.AccessGuarded.
	// Must be empty for the other two levels.
	Resource model.Resource
}
