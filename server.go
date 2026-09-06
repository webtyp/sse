//go:build !wasm

package sse

import (
	"webtyp.com/router"
)

// SSEServer handles Server-Sent Events streaming connections.
type SSEServer struct {
	tinySSE *tinySSE
	config  *ServerConfig
	hub     *hub
}

// Server creates a new SSEServer instance.
func (t *tinySSE) Server(c *ServerConfig) *SSEServer {
	return &SSEServer{
		tinySSE: t,
		config:  c,
		hub:     newHub(t, c),
	}
}

// StreamHandler returns a router.StreamFunc that serves SSE to a connected Streamer.
// Register it with: r.Stream(path, server.StreamHandler())
func (s *SSEServer) StreamHandler() router.StreamFunc {
	return func(st router.Streamer) {
		// 1. Resolve channels
		var channels []string
		var err error

		if s.config.ChannelProvider != nil {
			channels, err = s.config.ChannelProvider.ResolveChannels(st)
		} else {
			st.WriteStatus(500)
			st.Write([]byte("channel provider not configured\n")) //nolint:errcheck
			return
		}

		if err != nil {
			st.WriteStatus(401)
			st.Write([]byte(err.Error() + "\n")) //nolint:errcheck
			return
		}

		// 2. Set SSE headers
		st.SetHeader("Content-Type", "text/event-stream")
		st.SetHeader("Cache-Control", "no-cache")
		st.SetHeader("Connection", "keep-alive")

		// 3. Flush headers so the client knows the connection is open
		st.WriteStatus(200)
		st.Flush()

		// 4. Create client connection
		client := &clientConnection{
			channels: channels,
			send:     make(chan []byte, s.config.ClientChannelBuffer),
		}

		// Handle Last-Event-ID for replay
		lastEventID := st.GetHeader("Last-Event-ID")

		s.hub.register <- registerRequest{
			client:      client,
			lastEventID: lastEventID,
		}

		// Ensure unregister on exit
		defer func() {
			s.hub.unregister <- client
		}()

		// 5. Loop: push messages until the client disconnects (Write error) or hub closes send
		for msg := range client.send {
			if _, err := st.Write(msg); err != nil {
				return
			}
			st.Flush()
		}
	}
}

// Publish sends data to a single channel.
func (s *SSEServer) Publish(data []byte, channel string) {
	s.hub.broadcast <- &broadcastMessage{
		msg: &SSEMessage{
			Event: "", // Default
			Data:  data,
		},
		channels: []string{channel},
	}
}

// PublishEvent implements SSEPublisher.PublishEvent.
func (s *SSEServer) PublishEvent(event string, data []byte, channels ...string) {
	s.hub.broadcast <- &broadcastMessage{
		msg: &SSEMessage{
			Event: event,
			Data:  data,
		},
		channels: channels,
	}
}
